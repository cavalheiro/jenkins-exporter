package main

import (
	"crypto/tls"
	"database/sql"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/bndr/gojenkins"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

var db *sql.DB
var config Config
var jenkinsCli *gojenkins.Jenkins

var jenkinsRunningBuild = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Name: "jenkins_running_build",
	Help: "1 if there is a build running, 0 otherwise",
}, []string{"jobname", "buildid", "isgood"})

var jenkinsRunningBuildPipelineStatus = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Name: "jenkins_running_build_pipeline_status",
	Help: "0 if pipeline stage has failed, 1 if succeeded",
}, []string{"jobname", "buildid", "id", "stage"})

var jenkinsRunningBuildElapsedTime = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Name: "jenkins_running_build_elapsed_time",
	Help: "elapsed time of the current (running) build",
}, []string{"jobname", "buildid", "isgood"})

var jenkinsCompletedBuildSuccess = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Name: "jenkins_build_success",
	Help: "0 if build has failed, 1 if succeeded",
}, []string{"jobname", "buildid"})

var jenkinsCompletedBuildDurationSeconds = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Name: "jenkins_build_duration_seconds",
	Help: "Duration of the build in seconds",
}, []string{"jobname", "buildid"})

var jenkinsCompletedBuildTimestamp = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Name: "jenkins_build_timestamp",
	Help: "Timestamp of the build",
}, []string{"jobname", "buildid"})

var jenkinsCompletedBuildTestCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Name: "jenkins_build_test_count",
	Help: "Number of failed tests in the build",
}, []string{"jobname", "buildid", "result"})

var jenkinsCompletedBuildTestCaseFailureAge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Name: "jenkins_build_test_case_failure_age",
	Help: "Age of the failed tests in this build",
}, []string{"jobname", "buildid", "suite", "case", "status", "failedsince"})

var jenkinsCompletedBuildPipelineDurationSeconds = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Name: "jenkins_build_pipeline_duration_seconds",
	Help: "Duration of each pipeline stage in seconds",
}, []string{"jobname", "buildid", "id", "stage"})

//Register metrics with Prometheus client
func init() {
	prometheus.MustRegister(jenkinsRunningBuild)
	prometheus.MustRegister(jenkinsRunningBuildElapsedTime)
	prometheus.MustRegister(jenkinsRunningBuildPipelineStatus)
	prometheus.MustRegister(jenkinsCompletedBuildSuccess)
	prometheus.MustRegister(jenkinsCompletedBuildDurationSeconds)
	prometheus.MustRegister(jenkinsCompletedBuildTimestamp)
	prometheus.MustRegister(jenkinsCompletedBuildTestCount)
	prometheus.MustRegister(jenkinsCompletedBuildPipelineDurationSeconds)
	prometheus.MustRegister(jenkinsCompletedBuildTestCaseFailureAge)
}

// Load configuration
func init() {
	debugFlag := flag.Bool("debug", false, "Sets log level to debug.")
	configFileFlag := flag.String("config", "./config.toml", "Path to config file")
	flag.Parse()
	// Setting logger to debug level when debug flag was set.
	if *debugFlag == true {
		log.SetLevel(log.DebugLevel)
	}
	// load config
	if _, err := os.Stat(*configFileFlag); err == nil {
		if config, err = loadConfig(*configFileFlag); err != nil {
			log.Fatalf("Unable to parse configuration file: %s", err)
		}
	} else {
		log.Fatal("Please provide a config file with `-config <yourconfig>` or just create `config.toml` in this directory")
	}
	// Make sure update interval has a default value
	log.Debugf("Configuration: %+v", config)
	if config.Jenkins.UpdateInterval <= 0 {
		config.Jenkins.UpdateInterval = 1800 // 30 mins
	}
}

// Fetch metrics from Jenkins API
func updateMetrics() {
	log.Debugf("Connecting to Jenkins API and collecting metrics...")

	// Connect to Jenkins
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}
	if config.Jenkins.User != "" {
		jenkinsCli = gojenkins.CreateJenkins(client, config.Jenkins.URL, config.Jenkins.User, config.Jenkins.Password)
	} else {
		jenkinsCli = gojenkins.CreateJenkins(client, config.Jenkins.URL)
	}
	defer client.CloseIdleConnections()
	_, err := jenkinsCli.Init()
	if err != nil {
		log.Errorf("Unable to connect to Jenkins", err)
		return
	}

	// Reset all metrics
	jenkinsRunningBuild.Reset()
	jenkinsRunningBuildElapsedTime.Reset()
	jenkinsRunningBuildPipelineStatus.Reset()
	jenkinsCompletedBuildSuccess.Reset()
	jenkinsCompletedBuildDurationSeconds.Reset()
	jenkinsCompletedBuildTestCount.Reset()
	jenkinsCompletedBuildPipelineDurationSeconds.Reset()
	jenkinsCompletedBuildTestCaseFailureAge.Reset()
	jenkinsCompletedBuildTimestamp.Reset()

	/*
		------------------------------
		Iterate over configured jobs
		------------------------------
	*/

	for _, jobname := range config.Jenkins.Jobs {

		job, err := jenkinsCli.GetJob(jobname)
		if err != nil {
			log.Errorf("Job Does Not Exist", err)
			return
		}

		// Get Last Completed build
		lastCompletedBuild, err := job.GetLastCompletedBuild()
		if err != nil {
			log.Errorf("Unable to collect metrics for job: "+jobname+" - unable to get Last Completed Build", err)
			return
		}
		// Get Last Build (can be a running build)
		lastBuild, err := job.GetLastBuild()
		if err != nil {
			log.Errorf("Unable to collect metrics for job: "+jobname+" - unable to get Last Build", err)
			return
		}

		// Common labels to various metrics
		commonArgs := []string{
			job.GetName(),
			strconv.Itoa(int(lastCompletedBuild.GetBuildNumber())),
		}

		// Simple metrics - build timestamp and duration
		jenkinsCompletedBuildDurationSeconds.WithLabelValues(commonArgs...).Set(float64(lastCompletedBuild.GetDuration() / 1000))
		jenkinsCompletedBuildTimestamp.WithLabelValues(commonArgs...).Set(float64(lastCompletedBuild.GetTimestamp().Local().Unix()))

		// Simple metrics - test counts
		resultset, err := lastCompletedBuild.GetResultSet()
		jenkinsCompletedBuildTestCount.WithLabelValues(append(commonArgs, "fail")...).Set(float64(resultset.FailCount))
		jenkinsCompletedBuildTestCount.WithLabelValues(append(commonArgs, "skip")...).Set(float64(resultset.SkipCount))
		jenkinsCompletedBuildTestCount.WithLabelValues(append(commonArgs, "pass")...).Set(float64(resultset.PassCount))

		// Is there any build running?
		isRunning := func(running bool, err error) float64 {
			if running {
				return 1
			}
			return 0
		}(job.IsRunning())

		// Is the build good (without errors so far)?
		isGood := func(isGood bool) string {
			if isGood {
				return "1"
			}
			return "0"
		}(lastBuild.IsGood())
		jenkinsRunningBuild.WithLabelValues(
			job.GetName(),
			strconv.Itoa(int(lastBuild.GetBuildNumber())),
			isGood,
		).Set(isRunning)

		// If there is a job running, add metric with elapsed time
		if isRunning == 1 {
			var elapsedTime int64 = 0
			livePipe, _ := job.GetPipelineRun(strconv.Itoa(int(lastBuild.GetBuildNumber())))
			for _, stage := range livePipe.Stages {
				elapsedTime += stage.Duration / 1000
				jenkinsRunningBuildPipelineStatus.WithLabelValues(
					job.GetName(),
					strconv.Itoa(int(lastBuild.GetBuildNumber())),
					fmt.Sprintf("%03s", stage.ID),
					stage.Name).Set(
					func() float64 {
						switch stage.Status {
						case "SUCCESS":
							return 0
						case "IN_PROGRESS":
							return 1
						case "UNSTABLE":
							return 2
						case "FAILED":
							return 3
						}
						return -1
					}())
			}

			jenkinsRunningBuildElapsedTime.WithLabelValues(
				job.GetName(),
				strconv.Itoa(int(lastBuild.GetBuildNumber())),
				isGood,
			).Set(float64(elapsedTime))
		}

		// Build result
		jenkinsCompletedBuildSuccess.WithLabelValues(commonArgs...).Set(
			func(result string) float64 {
				if result == "FAILURE" {
					return 0
				}
				return 1
			}(lastCompletedBuild.GetResult()))

		// Iterate over failed and regression tests
		for _, suite := range resultset.Suites {
			for _, testcase := range suite.Cases {
				if !testcase.Skipped && testcase.Status != "PASSED" {
					jenkinsCompletedBuildTestCaseFailureAge.WithLabelValues(
						append(commonArgs,
							suite.Name,
							testcase.Name,
							testcase.Status,
							strconv.Itoa(int(testcase.FailedSince)),
						)...).Set(float64(testcase.Age))
				}
			}
		}

		// Last completed pipeline build duration
		lastCompletedPipeline, err := job.GetPipelineRun(strconv.Itoa(int(lastCompletedBuild.GetBuildNumber())))
		for _, stage := range lastCompletedPipeline.Stages {
			jenkinsCompletedBuildPipelineDurationSeconds.WithLabelValues(
				job.GetName(),
				strconv.Itoa(int(lastCompletedBuild.GetBuildNumber())),
				fmt.Sprintf("%03s", stage.ID),
				stage.Name,
			).Set(float64(stage.Duration / 1000))
		}
		log.Debugf("Finished collecting metrics for job: %s", jobname)
	}
}

func main() {

	// Poll Jenkins API on a regular interval
	go func() {
		for {
			updateMetrics()
			time.Sleep(time.Duration(config.Jenkins.UpdateInterval) * time.Second)
		}
	}()

	// Start http requests
	http.Handle("/metrics", promhttp.Handler())
	log.Info("Serving metrics on :9118/metrics")
	log.Fatal(http.ListenAndServe(":9118", nil))

}
