package cronify

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/docker/docker/client"
	"github.com/gorhill/cronexpr"
)

const (
	JobTypeKill    = "kill"
	JobTypeExec    = "exec"
	JobTypeStart   = "start"
	JobTypeStop    = "stop"
	JobTypeRestart = "restart"
)

// JobTypeMap maps given label value to the correct (internal) type
var JobTypeMap = map[string]string{
	"kill":    JobTypeKill,
	"signal":  JobTypeKill,
	"exec":    JobTypeExec,
	"start":   JobTypeStart,
	"stop":    JobTypeStop,
	"restart": JobTypeRestart,
}

// ParseJobs discover job configs by docker labels
func ParseJobs(labels map[string]string, containerID string) map[string]*Job {

	jobs := map[string]*Job{}

	for key, val := range labels {
		// parse only cronify specific labels
		if strings.HasPrefix(key, "cronify.") {
			// default error message
			unknownLabel := fmt.Sprintf("Ignore unknown label: %s\n", key)

			label := strings.Split(key, ".")
			if len(label) < 3 {
				log.Print(unknownLabel)
				continue
			}
			// second part is the jobname
			jobName := label[1]

			job, ok := jobs[jobName]
			if !ok {
				// initialize an new job
				job = &Job{
					JobName:     jobName,
					containerID: containerID,
					Run: &JobTypeConfig{
						Container: containerID,
					},
					Fail:    map[string]*JobTypeConfig{},
					Success: map[string]*JobTypeConfig{},
				}
				jobs[jobName] = job
			}

			var (
				configTrigger      string
				configTriggerIndex string
				configTriggerKey   string
			)
			switch len(label) {
			case 3:
				// cronify.JOBNAME.CONFIG
				key := label[2]
				if key == "schedule" {
					job.Schedule = val
					continue
				}
				err := parseJobConfig(job.Run, key, val)
				if err != nil {
					log.Println(err)
					continue
				}
			case 4:
				// cronify.JOBNAME.fail/success.CONFIG == cronify.JOBNAME.fail/success.default.CONFIG
				configTrigger = label[2]
				configTriggerIndex = "default"
				configTriggerKey = label[3]
				fallthrough
			case 5:
				// allow multiple notify jobs, this helps to interact with multiple containers differently
				// cronify.JOBNAME.fail/success.INDEX.CONFIG

				// if not fallthrough
				if configTriggerIndex == "" {
					configTrigger = label[2]
					configTriggerIndex = label[3]
					configTriggerKey = label[4]
				}

				// Check and initiate an job config for the trigger
				var configTriggerJob *JobTypeConfig
				switch strings.ToLower(configTrigger) {
				case "fail":
					configTriggerJob, ok = job.Fail[configTriggerIndex]
					if !ok {
						configTriggerJob = new(JobTypeConfig)
						configTriggerJob.Container = containerID
						job.Fail[configTriggerIndex] = configTriggerJob
					}
				case "success":
					configTriggerJob, ok = job.Success[configTriggerIndex]
					if !ok {
						configTriggerJob = new(JobTypeConfig)
						configTriggerJob.Container = containerID
						job.Success[configTriggerIndex] = configTriggerJob
					}
				default:
					log.Print(unknownLabel)
					continue
				}
				err := parseJobConfig(configTriggerJob, configTriggerKey, val)
				if err != nil {
					log.Println(err)
					continue
				}
			default:
				log.Print(unknownLabel)
				continue
			}
		}
	}
	return jobs
}

func parseJobConfig(jc *JobTypeConfig, key string, value string) error {
	switch strings.ToLower(key) {
	case "type":
		jobType, ok := JobTypeMap[strings.ToLower(value)]
		if !ok {
			return fmt.Errorf("invalid job type given '%s': '%s'", key, value)
		}
		jc.Type = jobType
	case "wait":
		switch strings.ToLower(value) {
		case "true":
			jc.Wait = true
		case "false":
			jc.Wait = false
		default:
			return fmt.Errorf("invalid boolean given for '%s': '%s'", key, value)
		}
	case "signal":
		// Signal to send to the container as an integer or string (e.g. SIGINT)
		jc.Signal = value
	case "command":
		if strings.HasPrefix(value, "[") {
			// seems to be an json formatted array
			err := json.Unmarshal([]byte(value), &jc.Command)
			if err != nil {
				return fmt.Errorf("could not parse json array from label '%s': %s. %v", value, key, err)
			}
		} else {
			jc.Command = strings.Split(value, " ")
		}
	case "timeout":
		duration, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("could not parse duration from label '%s': %v", key, err)
		}
		jc.Timeout = duration
	case "container":
		jc.Container = value
	default:
		return fmt.Errorf("unknown config label given: %s", key)
	}
	return nil
}

// Job represent the full job
type Job struct {
	containerID string
	JobName     string
	// Schedule is currently implemented as cron syntax
	Schedule string
	Run      *JobTypeConfig
	Fail     map[string]*JobTypeConfig
	Success  map[string]*JobTypeConfig
	// datetime of last run
	lastRun time.Time
	// datetime of next run
	nextRun time.Time
	// flag to avoid concurrent runs
	active bool
}

func (j *Job) scheduleNextRun() error {

	cron, err := cronexpr.Parse(j.Schedule)
	if err != nil {
		// set to Zero time
		j.nextRun = time.Time{}
		return err
	}

	j.nextRun = cron.Next(time.Now())
	log.Printf("Next run of job '%s' for container '%s' scheduled at %s", j.JobName, j.containerID, j.nextRun.String())
	return nil
}

func (j *Job) shouldRun() bool {
	return !j.active && !j.nextRun.IsZero() && time.Now().After(j.nextRun)
}

// JobTypeConfig configures the job/trigger to run
type JobTypeConfig struct {
	// Timeout this job after the duration
	Timeout time.Duration
	// Type of this job
	Type string
	// Wait for an command to finish (daemon?). Used for start.
	Wait bool
	// Signal to send if type is kill
	Signal string
	// Command to execute if type is exec
	Command []string
	// Container in context to run the job
	// send signal to this container, start/stop/restart this container or exec command in this container
	Container string
}

// NewExecution create a new execution wrapper
func (j *JobTypeConfig) NewExecution(dockerClient *client.Client) (JobExecution, error) {
	var exec JobExecution
	switch j.Type {
	case JobTypeKill:
		exec = &JobDockerKill{
			Client: dockerClient,
			Config: j,
		}
	case JobTypeStart:
		exec = &JobDockerStart{
			Client: dockerClient,
			Config: j,
		}
	case JobTypeStop:
		exec = &JobDockerStop{
			Client: dockerClient,
			Config: j,
		}
	case JobTypeRestart:
		exec = &JobDockerRestart{
			Client: dockerClient,
			Config: j,
		}
	case JobTypeExec:
		exec = &JobDockerExec{
			Client: dockerClient,
			Config: j,
		}
	default:
		return exec, fmt.Errorf("unknown job type: %s", j.Type)
	}
	return exec, nil
}
