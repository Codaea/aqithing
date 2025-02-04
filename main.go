package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-co-op/gocron/v2"
	"github.com/joho/godotenv"
)

var aqi int

func main() {
	// load .env to enviroment variables
	err := godotenv.Load()
	if err != nil {
		log.Default().Println("Error loading .env file. Does it exist?")
	}

	scheduler, err := gocron.NewScheduler()
	if err != nil {
		panic(err)
	}
	defer scheduler.Shutdown()

	// scrape hourly job
	_, err = scheduler.NewJob(
		gocron.DurationJob(
			1*time.Hour, // hourly job
		),
		gocron.NewTask(
			scrapeTask,
		),
		gocron.JobOption(gocron.WithStartDateTime(time.Now().Truncate(time.Hour).Add(time.Hour))), // start at the next hour
	)
	if err != nil {
		panic(err)
	}

	// inital scrape now, for immediate data
	scrapeTask()
	scheduler.Start()

	// webserver for aqi
	router := gin.Default()
	router.GET("/aqi", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"aqi": aqi,
		})
	})
	router.Run() // listen and serve on 0.0.0.0:8080 (for windows "localhost:8080")
}

type Response struct {
	Api_version    string `json:"api_version"`
	Timestamp      string `json:"timestamp"`
	Data_timestamp string `json:"data_timestamp"`
	Sensor         struct {
		Sensor_index string `json:"sensor_index"`
		Stats        struct {
			Pm2_5        float64 `json:"pm2.5"`
			Pm2_5_24hour float64 `json:"pm2.5_24hour"`
			Timestamp    string  `json:"time_stamp"`
		} `json:"stats"`
	} `json:"sensor"`
}

func scrapeTask() {
	sensorId := os.Getenv("SENSOR_ID")
	apiKey := os.Getenv("API_KEY")

	// get purple air pm2.5 24 hour average for sensor
	// build request
	client := http.Client{}
	req, err := http.NewRequest("GET", fmt.Sprintf("https://api.purpleair.com/v1/sensors/%s?fields=pm2.5_24hour", sensorId), nil)
	req.Header.Add("X-API-Key", apiKey)
	if err != nil {
		panic(err)
	}

	// send request
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	// parse response
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(content))

	var response Response
	json.Unmarshal(content, &response)

	fmt.Println(response.Sensor.Stats.Pm2_5_24hour)
	// set aqi for gin
	aqi = pm2_5ToAqi(response.Sensor.Stats.Pm2_5_24hour)
	fmt.Println(aqi) // maybe wrong?
}

// convert pm2.5 to aqi using USA EPA standard. see here https://document.airnow.gov/technical-assistance-document-for-the-reporting-of-daily-air-quailty.pdf
func pm2_5ToAqi(pm2_5 float64) int {

	/* the calcuation is

	   i = iHigh - iLow / bphigh - bpLow * (c - bpLow) + ilow

	   where
	   i is aqi we are solving for
	   iHigh is the high end of the aqi range from the table
	   iLow is the low end of the aqi range from the table
	   bphigh is the high end of the pm2.5 range from the table
	   bpLow is the low end of the pm2.5 range from the table

	   c is the pm2.5 value we are converting to aqi
	*/

	// find matching breakpoint
	pm2_5 = math.Round(pm2_5*10) / 10
	switch {
	case 0 <= pm2_5 && pm2_5 <= 9:
		return eval(0, 50, 0, 9, pm2_5)
	case 9.1 <= pm2_5 && pm2_5 <= 35.4:
		return eval(51, 100, 9.1, 35.4, pm2_5)
	case 35.5 <= pm2_5 && pm2_5 <= 55.4:
		return eval(101, 150, 35.5, 55.4, pm2_5)
	case 55.5 <= pm2_5 && pm2_5 <= 125.4:
		return eval(151, 200, 55.5, 125.4, pm2_5)
	case 125.5 <= pm2_5 && pm2_5 <= 225.4:
		return eval(201, 300, 125.5, 225.4, pm2_5)
	case 225.5 <= pm2_5:
		return eval(301, 500, 225.5, 500, pm2_5) // todo: fix highend handling out of range. listed as edge case on doc
	default:
		log.Default().Println("Error: pm2.5 value out of range")
		return 6666
	}

}

// converts pm2.5 to aqi using the formula i = iHigh - iLow / bphigh - bpLow * (c - bpLow) + ilow
// iLow is low end of aqi range, iHigh is high end of aqi range
// bpLow is low end of matter particulate breakpoint range, bpHigh is high end of matter particulate breakpoint range
func eval(iLow int, iHigh int, bpLow float64, bpHigh float64, pm2_5 float64) int { // todo, trunc this mofof
	return int(math.Round(float64(iHigh-iLow)/(bpHigh-bpLow)*(pm2_5-bpLow) + float64(iLow))) // all wrapped in round to get int
}
