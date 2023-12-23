package main

import (
	"internal/influxdbapi"
	"internal/system_metrics"
	"log"
	"sync"
	"time"

	"github.com/schollz/progressbar/v3"
)

func Fill(flags FillArgs) error {
	var influxDBApi = influxdbapi.NewInfluxDBApi(flags.DBArgs.Token, flags.DBArgs.Host, flags.DBArgs.Port, flags.DBArgs.Org, flags.DBArgs.Bucket, flags.DBArgs.Measurement)
	defer influxDBApi.Close()

	log.Printf("Filling database with metrics from %v files\n", len(flags.Files))
	bar := progressbar.Default(int64(len(flags.Files)), "Processing files")
	var wg sync.WaitGroup
	for _, file := range flags.Files {
		wg.Add(1)

		go func(filePath string, bar *progressbar.ProgressBar) error {
			defer wg.Done()

			id := GetIdFromFileName(filePath)
			bar.Describe("Reading file " + filePath)
			metric, _ := system_metrics.ReadFromFile(filePath, id)
			bar.Add(1)
			bar.Describe("Slicing metrics")

			// Slice the metric between startAt and duration
			// If the parameters are 0, it will return all metrics, so we don't need to check for that
			metric.SliceBetween(flags.StartAt, flags.Duration)

			progressChan := make(chan int)
			defer close(progressChan)

			bar.ChangeMax(bar.GetMax() + len(metric.Metrics))

			if len(flags.Anomaly) > 0 {
				bar.Describe("Injecting anomaly")
				if err := injectAnomaly(metric, flags.Anomaly); err != nil {
					return err
				}
			}

			go func() {
				for range progressChan {
					bar.Add(1)
				}
			}()

			bar.Describe("Writing metrics to database")
			influxDBApi.WriteMetrics(*metric, flags.Gap, func() {
				progressChan <- 1
			})
			return nil

		}(file, bar)
	}
	wg.Wait()
	bar.Finish()
	log.Println("Finished filling database")
	return nil
}

func Stream(flags StreamArgs) error {
	var influxDBApi = influxdbapi.NewInfluxDBApi(flags.DBArgs.Token, flags.DBArgs.Host, flags.DBArgs.Port, flags.DBArgs.Org, flags.DBArgs.Bucket, flags.DBArgs.Measurement)
	id := GetIdFromFileName(flags.File)

	insertTime := time.Now()

	// If append is set we need to get the last metric and start from there
	// else we start from now
	// If we start from now make sure the timemultiplier is set to 1
	if flags.Append {
		lastMetric, err := influxDBApi.GetLastMetric(id)
		if err != nil {
			return err
		}

		insertTime = time.Unix(lastMetric.Timestamp, 0)
	} else if flags.TimeMultiplier > 1 {
		log.Fatal("Timemultiplier can only be set while appending")
	}

	metrics, err := system_metrics.ReadFromFile(flags.File, id)
	if err != nil {
		return err
	}
	metrics.SliceBetween(flags.Startat, flags.Duration)
	if err := injectAnomaly(metrics, flags.Anomaly); err != nil {
		return err
	}

	// If we are appending we need to calculate the time delta between the first two metrics
	var timeDelta int64 = 0
	if flags.Append {
		if len(metrics.Metrics) < 2 {
			log.Println("Not enough metrics to calculate time delta, exiting...")
			return nil
		}
		timeDelta = (metrics.Metrics[1].Timestamp - metrics.Metrics[0].Timestamp)
		insertTime = insertTime.Add(time.Duration(timeDelta) * time.Second)
	}
	// Insert all metrics except the last one
	for i, metric := range metrics.Metrics[:len(metrics.Metrics)-1] {
		if insertTime.After(time.Now()) {
			log.Println("You have exceeded the current time. The time multiplier might be too high, exiting...")
			return nil
		}
		influxDBApi.WriteMetric(*metric, id, insertTime)
		log.Printf("%v: metric written at %v\n", id, insertTime.Format(time.RFC3339))

		timeDelta = (metrics.Metrics[i+1].Timestamp - metric.Timestamp)
		insertTime = insertTime.Add(time.Duration(timeDelta) * time.Second)

		time.Sleep((time.Second * time.Duration(timeDelta)) / time.Duration(flags.TimeMultiplier))
	}
	// Handle the last metric
	influxDBApi.WriteMetric(*metrics.Metrics[len(metrics.Metrics)-1], id, insertTime)
	log.Printf("%v: metric written at %v\n", id, insertTime.Format(time.RFC3339))

	return nil
}

func Clean(flags CleanArgs) error {
	var influxDBApi = influxdbapi.NewInfluxDBApi(flags.DBArgs.Token, flags.DBArgs.Host, flags.DBArgs.Port, flags.DBArgs.Org, flags.DBArgs.Bucket, flags.DBArgs.Measurement)

	defer influxDBApi.Close()

	if flags.All { // Clean the bucket
		return influxDBApi.DeleteBucket(flags.Startat)
	}

	var wg sync.WaitGroup
	for _, host := range flags.Hosts {
		wg.Add(1)
		go func(hostName string) {
			defer wg.Done()
			influxDBApi.DeleteHost(hostName, flags.Startat)
		}(host)
	}
	wg.Wait()

	return nil
}
