package main

import (
	"internal/influxdbapi"
	"internal/system_metrics"
	"log"
	"net/http"
	"os"
	"os/exec"
	"reflect"

	"github.com/gocarina/gocsv"

	"github.com/gin-gonic/gin"
)

var inProgress = false

func triggerDetection(ctx *gin.Context) {
	log.Println("Anomaly detection request received!")
	dbapi := influxdbapi.NewInfluxDBApi(os.Getenv("INFLUXDB_TOKEN"), os.Getenv("INFLUXDB_HOST"), os.Getenv("INFLUXDB_PORT"), os.Getenv("INFLUXDB_ORG"), os.Getenv("INFLUXDB_BUCKET"), "metrics")
	defer dbapi.Close()
	algorithm := ctx.Param("algorithm")
	host := ctx.Param("host")
	duration := ctx.Param("duration")
	//These checks might be in simba instead?
	if host == "" {
		ctx.String(http.StatusOK, "Host field is empty")
		log.Println("Host field is empty")
		return
	}
	if duration == "" {
		ctx.String(http.StatusOK, "Duration field is empty")
		log.Println("Duration field is empty")
		return
	}
	if inProgress {
		ctx.String(http.StatusOK, "Anomaly detection is already in progress")
		log.Println("Anomaly detection is already in progress")
		return
	}
	detection, exists := supportedAlgorithms[algorithm]
	if !exists {
		ctx.String(http.StatusOK, "Algorithm %v is not supported", algorithm)
		log.Printf("Algorithm %v is not supported", algorithm)
		return
	}
	inProgress = true

	log.Printf("Starting anomaly detection for %v\n", host)
	parameters, err := NewAnomalyDetection(dbapi, host, duration)
	if err != nil {
		ctx.String(http.StatusOK, "%v", err)
		inProgress = false
		return
	}

	go func() {
		defer func() {
			inProgress = false
		}()

		anomalies, err := detection(parameters)
		if err != nil {
			log.Printf("Anomaly detection failed with: %v\n", err)
			return
		}
		log.Println("Logging anomalies to file")
		if err = logAnomalies("/tmp/anomalies.csv", host, algorithm, *anomalies); err != nil {
			log.Printf("Error when writing anomalies to file: %v\n", err)
			return
		}
		dbapi.Measurement = "anomalies"
		log.Println("Writing anomalies to influxdb")
		if err = dbapi.WriteAnomalies(*anomalies, host, algorithm); err != nil {
			log.Printf("Error when writing anomalies to influxdb: %v\n", err)
			return
		}
		log.Println("Anomaly detection is done!")
	}()

	ctx.String(http.StatusOK, "Anomaly detection triggered!\n")
}

// Runs "testyp.py" and prints the output
func pythonSmokeTest() {

	log.Println("Running python smoke test...")
	cmd := exec.Command("python", "./testpy.py", "Python is working!")

	//executes command, listends to stdout, puts w/e into "out" var unless error
	out, err := cmd.Output()
	if err != nil {
		log.Fatal(err)
	}
	//Print, Need explicit typing or it prints an array with unicode numbers
	log.Print(string(out))
	log.Println("Python smoke test complete!")
}

/*
Takes AnomalyMetric struct and writes it to a log file
Logfile output: [time, host, metric, comment]
Returns error if something fails
*/
func logAnomalies(filePath string, host string, algorithm string, data []system_metrics.AnomalyDetectionOutput) error {
	outputArray := []system_metrics.AnomalyEvent{}
	for _, v := range data {
		r := reflect.ValueOf(v)
		for i := 1; i < r.NumField(); i++ {
			if r.Field(i).Interface() == true {
				outputArray = append(outputArray, system_metrics.AnomalyEvent{Timestamp: v.Timestamp, Host: host, Metric: r.Type().Field(i).Tag.Get("csv"), Comment: algorithm})
			}
		}
	}

	outputFile, err := os.Create(filePath)
	if err != nil {
		log.Printf("Error when creating file: %v", err)
		return err
	}
	defer outputFile.Close()
	err = gocsv.MarshalFile(&outputArray, outputFile)
	if err != nil {
		log.Printf("Error while parsing metrics from file: %v", err)
		return err
	}
	return nil
}

func checkEnv() {
	log.Println("Checking environment variables...")

	if _, exists := os.LookupEnv("INFLUXDB_HOST"); !exists {
		log.Fatal("INFLUXDB_HOST is not set")
	}
	if _, exists := os.LookupEnv("INFLUXDB_PORT"); !exists {
		log.Fatal("INFLUXDB_PORT is not set")
	}
	if _, exists := os.LookupEnv("INFLUXDB_TOKEN"); !exists {
		log.Fatal("INFLUXDB_TOKEN is not set")
	}
	if _, exists := os.LookupEnv("INFLUXDB_ORG"); !exists {
		log.Fatal("INFLUXDB_TOKEN is not set")
	}
	if _, exists := os.LookupEnv("INFLUXDB_BUCKET"); !exists {
		log.Fatal("INFLUXDB_TOKEN is not set")
	}
	log.Println("Environment variables are set!")
}

func main() {
	log.Println("Starting Nala...")
	pythonSmokeTest()
	checkEnv()

	router := gin.Default()

	router.GET("/nala/:algorithm/:host/:duration", triggerDetection)

	router.GET("/nala/test", func(ctx *gin.Context) {
		ctx.String(http.StatusOK, "Nala is working!")
	})

	router.GET("/nala/status", func(ctx *gin.Context) {
		responseText := ""
		if inProgress {
			responseText = "Anomaly detection in progress"
		} else {
			responseText = "No anomaly detection running"
		}
		ctx.String(http.StatusOK, responseText)
	})

	router.Run("0.0.0.0:8088")
}
