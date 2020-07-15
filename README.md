# Jenkins Metrics exporter

Jenkins Metrics exporter aims to extract metrics like build results, pipeline status and test information from Jenkins via http native API and expose them in Prometheus format.

## Metrics

```
# HELP jenkins_build_pipeline_duration_seconds Duration of each pipeline stage in seconds
# HELP jenkins_build_success 0 if build has failed, 1 if succeeded
# HELP jenkins_build_test_case_failure_age Age of the failed tests in this build
# HELP jenkins_build_test_count Number of failed tests in the build
# HELP jenkins_build_timestamp Timestamp of the build
# HELP jenkins_running_build 1 if there is a build running, 0 otherwise
# HELP jenkins_running_build_elapsed_time elapsed time of the current (running) build
# HELP jenkins_running_build_pipeline_status 0 if pipeline stage has failed, 1 if succeeded
```

## Building and runnning

Prerequisites:

- Go compiler
- Configure your Jenkins settings in the `config.toml` file.

Building:

```
git clone https://github.com/cavalheiro/jenkins-metrics.git
cd jenkins-metrics
go build .
```

Running:

`./jenkins-metrics -h` 

### Running as Docker container

Building the container:

`docker build -t jenkins-metrics .`

Running the container:

`docker run -d --name jenkins-metrics -p 9118:9118 -v <local-config-dir>:/go/etc jenkins-metrics`

Note: Create a local config dir <local-config-dir> and copy your config file (config.toml) there.

