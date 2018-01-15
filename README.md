# Cronify
Cronify discovers docker by labels for cron like jobs. Jobs are executed in the context of a container.

## Usage
The docker client has some extra env variables you can set: `DOCKER_CERT_PATH`, `DOCKER_TLS_VERIFY`, `DOCKER_HOST`

### Run in a docker container
[Image DockerHub](https://hub.docker.com/r/project0de/cronify)

Obviously docker socket is required for api calls to docker.

```bash
docker run -ti -v /var/run/docker.sock:/var/run/docker.sock project0de/cronify
```

## Docker Labels
To prevent automatic creation of unwanted rules, container needs to be labeled:

### cronify
The label `cronify` enables this container for cronify to use. The value needs to be set to `true`.

Cronify is designed to create multiple jobs to a container `cronify.<JOBNAME>.<CONFIG>`.

Furthemore its possible to have multiple success or fail jobs which can be configured in the same way like the casual job `cronify.<JOBNAME>.success/fail.<FAIL/SUCCESS NAME>.<CONFIG>`. This allows us to create a simple pipeline to run (for example cleanup jobs).

### Config

#### Schedule
Schedule is a **required** option which is only available on the main job.

Schedule this job in a cron expression.
See also https://en.wikipedia.org/wiki/Cron#Predefined_scheduling_definitions and https://github.com/gorhill/cronexpr#predefined-cron-expressions

```yaml
# Run once an hour at the beginning of the hour
cronify.<JOBNAME>.schedule: 0 0 * * * * *
```

#### Type
Type is a **required** option. One of the following types are currently implemented:

* `kill`: Sends a posix signal to the container. See also `signal`.
* `start`: Start a container.
* `stop`: Stop a container.
* `restart`: Restart a container.
* `exec`: Attach and execute a command to an container. Output is currently written to stdout of cronify daemon.

```yaml
# main job
cronify.<JOBNAME>.type: exec
# on success and/or fail
cronfiy.<JOBNAME>.success/fail.<NAME>.type: restart
```

#### Container
Container specifies the container id or name to run the job on.
Defaults to the container id where the labels are set on. If the container does not exist at run the job fails!

```yaml
# main job
cronify.<JOBNAME>.container: my_container_name
# on success and/or fail
cronfiy.<JOBNAME>.success/fail.<NAME>.container: my_cleanup_container
```

#### Signal
Currently only used for the `kill` command. Signal defaults to whatever docker uses by docker `kill` command (usually SIGKILL). This can be set to any posix signal: https://en.wikipedia.org/wiki/Signal_(IPC)#POSIX_signals.

See also https://github.com/krallin/tini for implementing signal forwarding in docker containers.

```yaml
# kill container on main job
cronify.<JOBNAME>.signal: SIGKILL
# notify reload process on success and/or fail
cronfiy.<JOBNAME>.success/fail.<NAME>.signal: SIGHUP
```


#### Command
Currently only used and **required** for the `exec` command. Executes the specified command. Need to return exit code `0` for success. Note: Docker commands are always specified as array!

```yaml
# Simple string, will be splitted by space char to an array!:
cronify.<JOBNAME>.command: /my/bin/or/script arg1 arg2
# or set more complex command as json formatted array:
# note: this is example provides a docker-compose yaml syntax!
cronfiy.<JOBNAME>.success/fail.<NAME>.signal: >
  ["/bin/sh", "-c", "echo 'run a job' && sleep 20 && echo 'job done' && exit 2"]
```

#### Timeout
Optional timeout for the job in golang duration syntax. job will be marked as fail if timout reaches.

```yaml
# Timeout main job after 60 seconds
cronify.<JOBNAME>.timeout: 60s
# second timeout on success/fail job (5 minutes)
cronfiy.<JOBNAME>.success/fail.<NAME>.timeout: 5m
```
