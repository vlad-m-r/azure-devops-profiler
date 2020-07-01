package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"
)

var filename string
var ADOToken string = os.Getenv("ADO_TOKEN")
var ADOUrl string = os.Getenv("ADO_URL")

func doRequest(organizationUrl string) []byte {
	var ioReader io.Reader
	var response *http.Response
	var httpError error
	var request *http.Request
	var reqError error

	// Start request
	request, reqError = http.NewRequest(http.MethodGet, organizationUrl, ioReader)

	if reqError != nil {
		log.Println("reqError: " + reqError.Error())
	}

	// Auth
	request.SetBasicAuth("", ADOToken)

	// Shoot
	response, httpError = http.DefaultClient.Do(request)

	if httpError != nil {
		log.Println("The HTTP request failed with error: " + httpError.Error())
	}

	if response != nil {
		bodyBytes, err := ioutil.ReadAll(response.Body)
		if err != nil {
			log.Println("Failed to read response body:" + response.Status)
		}
		defer response.Body.Close()
		return bodyBytes
	}

	return []byte{}
}

func getPoolAgentData(poolID string, logFile *os.File) {
	poolAgentURL := fmt.Sprintf("%s/_apis/distributedtask/pools/%s/agents?includeAssignedRequest=true", ADOUrl, poolID)
	bodyBytes := doRequest(poolAgentURL)

	var logFileOutput string
	var totalAgents int
	var activeAgents int
	var idleAgents int
	var enabledAgents int
	var disabledAgents int
	var onlineAgents int
	var offlineAgents int
	var agentUtilization float64

	var data map[string]interface{}
	if unmarshalError := json.Unmarshal(bodyBytes, &data); unmarshalError != nil {
		return
	}

	agentData := data["value"].([]interface{})

	for _, agentValues := range agentData {
		build := agentValues.(map[string]interface{})

		// Active agents
		if _, keyExists := build["assignedRequest"]; keyExists {
			activeAgents += 1
		}

		agentEnabled := build["enabled"].(bool)

		if agentEnabled {
			enabledAgents += 1
		} else {
			disabledAgents += 1
		}

		agentStatus := build["status"]

		if agentStatus == "online" {
			onlineAgents += 1
		} else {
			offlineAgents += 1
		}

		// Total agents
		totalAgents += 1
	}

	idleAgents = totalAgents - activeAgents
	agentUtilization = float64(activeAgents) / float64(enabledAgents) * 100

	logFileOutput += fmt.Sprintf("totalAgents: %v\n", totalAgents)
	logFileOutput += fmt.Sprintf("activeAgents: %v\n", activeAgents)
	logFileOutput += fmt.Sprintf("idleAgents: %v\n", idleAgents)
	logFileOutput += fmt.Sprintf("enabledAgents: %v\n", enabledAgents)
	logFileOutput += fmt.Sprintf("disabledAgents: %v\n", disabledAgents)
	logFileOutput += fmt.Sprintf("onlineAgents: %v\n", onlineAgents)
	logFileOutput += fmt.Sprintf("offlineAgents: %v\n", offlineAgents)
	logFileOutput += fmt.Sprintf("agentUtilization: %v\n", agentUtilization)
	_, _ = logFile.WriteString(logFileOutput)
}

func getPoolBuildData(poolID string, logFile *os.File) {

	poolBuildURL := fmt.Sprintf("%s/_apis/distributedtask/pools/%s/jobrequests", ADOUrl, poolID)
	bodyBytes := doRequest(poolBuildURL)

	var logFileOutput string
	var buildsInTheQueue int
	var buildsRunning int

	var data map[string]interface{}
	if unmarshalError := json.Unmarshal(bodyBytes, &data); unmarshalError != nil {
		fmt.Println("Unmarshal error")
		return
	}

	timeNow := time.Now()
	buildData := data["value"].([]interface{})

	for _, buildValues := range buildData {
		build := buildValues.(map[string]interface{})

		// Collect some parameters to find builds
		_, isAssigned := build["assignTime"]
		_, hasResult := build["result"]

		// Find builds in the queue: builds have no assignTime
		if !isAssigned {
			queueTime := build["queueTime"].(string)

			t, err := time.Parse(time.RFC3339, queueTime)
			if err != nil {
				fmt.Println(err)
			}

			diff := timeNow.Sub(t)
			buildsInTheQueue += 1
			logFileOutput += fmt.Sprintf("Build: %s - in the queue for %s. Full build data: %s\n", build["owner"].(map[string]interface{})["name"], diff, build)
		}

		// Find running jobs: Builds have been assigned but have no result
		if isAssigned && !hasResult {
			assignedTime := build["assignTime"].(string)

			t, err := time.Parse(time.RFC3339, assignedTime)
			if err != nil {
				fmt.Println(err)
			}

			diff := timeNow.Sub(t)
			buildsRunning += 1
			logFileOutput += fmt.Sprintf("Build: %s - running for %s. Full build data: %s\n", build["owner"].(map[string]interface{})["name"], diff, build)
		}
	}

	logFileOutput += fmt.Sprintf("Builds in the queue: %v\n", buildsInTheQueue)

	_, _ = logFile.WriteString(logFileOutput)
}

func main() {
	var pools map[string]string

	jsonFile, err := os.Open("pools.json")

	if err != nil {
		fmt.Println("Failed to find pools.json")
		panic(err)
	}

	defer jsonFile.Close()
	
	byteValue, _ := ioutil.ReadAll(jsonFile)
	_ = json.Unmarshal(byteValue, &pools)

	currentTime := time.Now()
	filename = currentTime.Format("2006-01-02_15-04-05")

	for poolID, poolName := range pools {
		fmt.Println("Checking pool:", poolName)

		var poolsLogPath = "pools/" + poolName

		if _, err := os.Stat(poolsLogPath); os.IsNotExist(err) {
			_ = os.Mkdir(poolsLogPath, os.ModeDir)
		}

		logFile, err := os.Create(poolsLogPath + "/" + filename)

		if err != nil {
			panic(err)
		}

		defer func() {
			if err := logFile.Close(); err != nil {
				panic(err)
			}
		}()

		_, _ = logFile.WriteString(fmt.Sprintf("Pool: %v\n", poolName))
		getPoolAgentData(poolID, logFile)
		getPoolBuildData(poolID, logFile)
		time.Sleep(5 * time.Second)
	}

}
