package cronify

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

// JobExecution represent a job to execute
type JobExecution interface {
	execute(ctx context.Context, outputWriter io.Writer) (bool, error)
}

// JobDockerKill implements JobExecution for sending process signals
type JobDockerKill struct {
	Client *client.Client
	Config *JobTypeConfig
}

func (j *JobDockerKill) execute(ctx context.Context, outputWriter io.Writer) (bool, error) {
	err := j.Client.ContainerStart(ctx, j.Config.Container, types.ContainerStartOptions{})
	if err != nil {
		return false, err
	}
	return true, nil
}

// JobDockerRestart restarts container
type JobDockerRestart struct {
	Client *client.Client
	Config *JobTypeConfig
}

func (j *JobDockerRestart) execute(ctx context.Context, outputWriter io.Writer) (bool, error) {
	err := j.Client.ContainerRestart(ctx, j.Config.Container, &j.Config.Timeout)
	if err != nil {
		return false, err
	}
	return true, nil
}

// JobDockerStop stops container
type JobDockerStop struct {
	Client *client.Client
	Config *JobTypeConfig
}

func (j *JobDockerStop) execute(ctx context.Context, outputWriter io.Writer) (bool, error) {
	err := j.Client.ContainerStop(ctx, j.Config.Container, &j.Config.Timeout)
	if err != nil {
		return false, err
	}
	return true, nil
}

// JobDockerStart starts container
type JobDockerStart struct {
	Client *client.Client
	Config *JobTypeConfig
}

func (j *JobDockerStart) execute(ctx context.Context, outputWriter io.Writer) (bool, error) {
	err := j.Client.ContainerStart(ctx, j.Config.Container, types.ContainerStartOptions{})
	if err != nil {
		return false, err
	}
	//todo: wait with log output attach -> j.Config.Wait
	return true, nil
}

// JobDockerExec implements JobExecution for attached container executions
type JobDockerExec struct {
	Client    *client.Client
	Config    *JobTypeConfig
	LogWriter io.Writer
}

func (j *JobDockerExec) execute(ctx context.Context, outputWriter io.Writer) (bool, error) {
	// decouple context...

	execID, err := j.Client.ContainerExecCreate(ctx, j.Config.Container, types.ExecConfig{
		AttachStderr: true,
		AttachStdout: true,
		Cmd:          j.Config.Command,
	})
	if err != nil {
		return false, err
	}

	hj, err := j.Client.ContainerExecAttach(ctx, execID.ID, types.ExecStartCheck{
		Detach: false,
	})
	if err != nil {
		return false, err
	}
	defer hj.Close()

	// start writer shipped by docker (StdCopy will demultiplex `src`, assuming that it contains two streams)
	go stdcopy.StdCopy(outputWriter, outputWriter, hj.Reader)

	err = j.Client.ContainerExecStart(ctx, execID.ID, types.ExecStartCheck{})
	if err != nil {
		return false, err
	}

	// wait for end of execution
	// todo: timeout
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		<-ticker.C
		inspect, err := j.Client.ContainerExecInspect(ctx, execID.ID)
		if err != nil {
			return false, err
		}
		if inspect.Running {
			continue
		}
		log.Print(inspect)
		if inspect.ExitCode != 0 {
			return false, fmt.Errorf("execution failed with exit code: %d", inspect.ExitCode)
		}
		break
	}

	return true, nil
}
