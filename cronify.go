package cronify

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/docker/docker/client"
)

// Cronify
type Cronify struct {
	DockerClient      *client.Client
	MaxConcurrentJobs int
	JobList           []*Job
	sync              sync.Mutex
}

func (c *Cronify) AddJob(job *Job) {
	c.sync.Lock()
	err := job.scheduleNextRun()
	if err != nil {
		log.Printf("Could not add job %s: %v", job.JobName, err)
		return
	}
	c.JobList = append(c.JobList, job)
	c.sync.Unlock()
}

func (c *Cronify) RemoveJobsByContainerID(containerID string) {
	c.sync.Lock()
	for i := 0; i < len(c.JobList); i++ {
		if c.JobList[i].containerID == containerID {
			c.JobList = append(c.JobList[:i], c.JobList[i+1:]...)
		}
	}
	c.sync.Unlock()
}

func (c *Cronify) nextJobs() []*Job {
	var jobs []*Job
	for _, j := range c.JobList {
		if j.shouldRun() {
			jobs = append(jobs, j)
		}
	}
	return jobs
}

// runJob handles the whole job flow
func (c *Cronify) runJob(ctx context.Context, job *Job) error {
	if job.active {
		return fmt.Errorf("job is still active")
	}
	job.active = true

	// start go routine
	go func(c *Cronify, ctx context.Context, job *Job) {
		//todo: decouple context here to cancel the workflow

		/// main run
		success, err := c.execute(ctx, job.Run)
		log.Printf("success: %v, error: %v\n", success, err)

		// success or fail runs
		var postJobs map[string]*JobTypeConfig
		if success {
			postJobs = job.Success
		} else {
			postJobs = job.Fail
		}

		for i, conf := range postJobs {
			log.Printf("start post job: %s", i)
			success, err := c.execute(ctx, conf)
			log.Printf("success: %v, error: %v\n", success, err)
		}

		///
		job.lastRun = job.nextRun
		if err := job.scheduleNextRun(); err != nil {
			log.Printf("Could not schedule job '%s' for '%s': %s\n", job.JobName, job.containerID, err.Error())
		}
		job.active = false
	}(c, ctx, job)

	return nil
}

// execute handles direct Run/Success/Fail executions
func (c *Cronify) execute(ctx context.Context, config *JobTypeConfig) (bool, error) {
	exec, err := config.NewExecution(c.DockerClient)
	if err != nil {
		return false, err
	}

	var (
		cancelCtx context.CancelFunc
		execCtx   context.Context
	)

	if config.Timeout.Seconds() > 0 {
		execCtx, cancelCtx = context.WithTimeout(ctx, config.Timeout)
	} else {
		execCtx, cancelCtx = context.WithCancel(ctx)
	}
	defer cancelCtx()

	return exec.execute(execCtx, os.Stdout)
}

// Start cronify main process
func (c *Cronify) Start() chan bool {
	stopped := make(chan bool, 1)
	ticker := time.NewTicker(1 * time.Second)
	// todo: implement context cancelation
	//var ctxs []context.Context

	go func() {
	Schedule:
		for {
			select {
			case <-ticker.C:
				jobs := c.nextJobs()
				if len(jobs) == 0 {
					continue
				}
				for _, job := range jobs {
					// concurrent run ?
					ctx := context.Background()
					err := c.runJob(ctx, job)
					if err != nil {
						log.Printf("Could not start job '%s' for '%s': %s\n", job.JobName, job.containerID, err.Error())
					}
				}
			case <-stopped:
				break Schedule
			}
		}
		// cancel every job
		//for _, ctx := range ctxs {
		//	ctx.Done()
		//}
	}()

	return stopped
}
