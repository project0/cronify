package main

import (
	"context"
	"flag"
	"log"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/project0/cronify"
)

func main() {

	DockerLabelFilter := flag.String("label", "cronify=true", "Discovery only container attached with this label")
	flag.Parse()

	// enforce older api version
	// os.Setenv("DOCKER_API_VERSION", "1.26")
	cl, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		panic(err)
	}

	cron := cronify.Cronify{
		DockerClient: cl,
	}

	containerFilterArgs := filters.NewArgs()
	if *DockerLabelFilter != "" {
		containerFilterArgs.Add("label", *DockerLabelFilter)
	}

	log.Println("Synchronize with docker host")
	container, err := cl.ContainerList(context.Background(), types.ContainerListOptions{
		Filters: containerFilterArgs,
		All:     true,
	})
	if err != nil {
		panic(err)
	}

	for _, c := range container {
		job := cronify.ParseJobs(c.Labels, c.ID)
		for _, j := range job {
			log.Printf("Add job for container %s\n", c.ID)
			cron.AddJob(j)
		}
	}

	stopChan := cron.Start()

	ctx := context.Background()

	containerFilterArgs.Add("event", "create")
	containerFilterArgs.Add("event", "update")
	containerFilterArgs.Add("event", "destroy")
	// watch rename?

	containerFilterArgs.Add("type", "container")
	chanEvents, chanErrors := cl.Events(ctx, types.EventsOptions{
		Filters: containerFilterArgs,
	})
	log.Println("Watch for container events")

	for {
		select {
		case event := <-chanEvents:
			switch event.Action {
			case "create":
				info, err := cl.ContainerInspect(context.Background(), event.Actor.ID)
				if err != nil {
					log.Println("failed to inspect")
					continue
				}
				job := cronify.ParseJobs(info.Config.Labels, info.ID)
				for _, j := range job {
					log.Printf("Add job for container %s\n", event.Actor.ID)

					cron.AddJob(j)
				}
			case "destroy":
				log.Printf("Remove jobs for container %s\n", event.Actor.ID)
				cron.RemoveJobsByContainerID(event.Actor.ID)
			default:
				log.Printf("Unhandled event action: %s", event.Action)
			}
		case errors := <-chanErrors:
			stopChan <- true
			log.Panic(errors.Error())
			return
		}
	}
}
