package router

import (
	"fmt"
	"os"
	grpc "solution/grpc"
	"time"

	"encoding/json"
	logger "solution/logger"

	"github.com/gofiber/fiber/v2"
)

// datatype for response sending to client
type Result struct {
	Alarm        bool
	Label        string
	Tagging_rate float32
}

type ResultWithSwitch struct {
	Alarm        bool
	Label        string
	Tagging_rate float32
	Switch       bool
}

// datatype for goroutine results
type ResultRoutine struct {
	Alarm        bool
	Label        string
	Tagging_rate float32
	Err          error
}

// for parsing client's request
type SignInCredentials struct {
	sound []byte
}

// datatype to round robin grpc connection
type RoundRobin struct {
	Index int
	Links []string
}

// variable to send requests as round robin
var Robin RoundRobin

// initialize round robin object
func init() {
	Robin = RoundRobin{
		Index: 0,
		Links: []string{
			"localhost:8080",
			"localhost:8080",
		},
	}
}

// round robin function.
// return url to send request, and prepare for next function call
func (Lsts *RoundRobin) next() string {
	fmt.Printf("%d\t", Lsts.Index)
	Lsts.Index += 1
	if Lsts.Index == len(Lsts.Links) {
		Lsts.Index = 0
	}
	return Lsts.Links[Lsts.Index]
}

// routing to check if ml server is connected
func PingPong(c *fiber.Ctx) error {
	logger.MyLogger.Printf("request from", c.IP())
	resultChan := make(chan bool)

	go func() {
		res := grpc.Ping(Robin.next())
		resultChan <- res
	}()
	res := <-resultChan
	if res {
		return c.SendString("Pong!")
	} else {
		return c.SendString("Unable to connect ML server")
	}
}

// routing to send sound format of bytes to ml server.
func MlServer(c *fiber.Ctx) error {
	logger.MyLogger.Printf("request from", c.IP())
	//parsing sound file from request
	body := c.Body()
	parsed := SignInCredentials{
		sound: []byte(body),
	}

	//because we use goroutine, we use channel to get result
	resultChan := make(chan ResultRoutine)

	//using goroutine, send request to ml server, and get response
	go func() {
		alarm, label, tagging_rate, err := grpc.GRPC(Robin.next(), parsed.sound)
		response := ResultRoutine{
			Alarm:        alarm,
			Label:        label,
			Tagging_rate: tagging_rate,
			Err:          err,
		}
		resultChan <- response
	}()

	//check result, and check if error exist
	res := <-resultChan
	if res.Err != nil {
		return c.SendString("GRPC error")
	}

	//if no error exist, parse the data as response type
	data, err := os.ReadFile("switch.json")
	if err != nil {
		return c.SendString("Error reading the file" + err.Error())
	}

	var labelSwitches map[string]bool
	err = json.Unmarshal(data, &labelSwitches)
	if err != nil {
		return c.SendString("Error parsing Json: " + err.Error())
	}

	switchVal, exists := labelSwitches[res.Label]
	if !exists {
		return c.SendString("Error: label not found in JSON switch file")
	}
	response := ResultWithSwitch{
		Alarm:        res.Alarm,
		Label:        res.Label,
		Tagging_rate: res.Tagging_rate,
		Switch:       switchVal,
	}

	u, err := json.Marshal(response)
	if err != nil {
		logger.MyLogger.Printf("[json] json parsing error")
		return c.SendStatus(400)
	}

	currentTime := time.Now()
	ResultWithTime := struct {
		ResultWithSwitch
		Timestamp string `json:"timestamp"`
	}{
		ResultWithSwitch: response,
		Timestamp:        currentTime.Format("15:04:05"), //time.RFC3339Nano
	}

	i, err := json.Marshal(ResultWithTime)
	if err != nil {
		logger.MyLogger.Printf("[json] json parsing error")
		return c.SendStatus(400)
	}
	// result.json 파일에 JSON 데이터를 덮어쓰기
	err = os.WriteFile("result.json", i, 0644)
	if err != nil {
		logger.MyLogger.Printf("[file] file writing error: %v", err)
		return c.SendStatus(500) // 서버 내부 오류 상태 코드 반환
	}

	return c.SendString(string(u))
}

// routing to send sound file as byte[] to ml server
func Files(c *fiber.Ctx) error {
	logger.MyLogger.Printf("request from", c.IP())
	//parsing sound file from request
	if form, err := c.MultipartForm(); err == nil {
		file := form.File["sounds"][0]
		logger.MyLogger.Print(file.Filename, file.Size, file.Header["Content-Type"][0])

		fileContent, _ := file.Open()
		var byteContainer []byte
		byteContainer = make([]byte, 1000000)
		fileContent.Read(byteContainer)

		//because we use goroutine, we use channel to get result
		resultChan := make(chan ResultRoutine)

		//using goroutine, send request to ml server, and get response
		go func() {
			alarm, label, tagging_rate, err := grpc.GRPC(Robin.next(), byteContainer)
			response := ResultRoutine{
				Alarm:        alarm,
				Label:        label,
				Tagging_rate: tagging_rate,
				Err:          err,
			}
			resultChan <- response
		}()

		//check result, and check if error exist
		res := <-resultChan
		if res.Err != nil {
			return c.SendString("GRPC error")
		}

		//if no error exist, parse the data as response type
		response := Result{
			Alarm:        res.Alarm,
			Label:        res.Label,
			Tagging_rate: res.Tagging_rate,
		}

		u, err := json.Marshal(response)
		if err != nil {
			logger.MyLogger.Printf("[json] json parsing error")
			return c.SendStatus(400)
		}
		return c.SendString(string(u))
	}
	logger.MyLogger.Printf("Multipart Form error")
	return c.SendStatus(400)
}

// Update the switch.json
func UpdateSwitch(c *fiber.Ctx) error {
	logger.MyLogger.Printf("request from", c.IP())

	//parsing switch from request
	var newSwitches map[string]bool
	err := c.BodyParser(&newSwitches)
	if err != nil {
		return c.SendString("Error parsing Json: " + err.Error())
	}

	// Read an existing switch.json file
	data, err := os.ReadFile("switch.json")
	if err != nil {
		return c.SendString("Error reading the file: " + err.Error())
	}

	// Parsing an existing setting
	var currentSwitches map[string]bool
	err = json.Unmarshal(data, &currentSwitches)
	if err != nil {
		return c.SendString("Error parsing Json: " + err.Error())
	}

	// Updating the switch.json file
	for key, value := range newSwitches {
		if _, exists := currentSwitches[key]; exists {
			currentSwitches[key] = value
		} else {
			// 존재하지 않는 키에 대해서는 에러 메시지를 반환
			return c.SendString("Error: Key '" + key + "' does not exist in the current switches")
		}
	}

	// 변경된 설정을 파일에 다시 씁니다.
	updatedData, err := json.Marshal(currentSwitches)
	if err != nil {
		return c.SendString("Error marshalling JSON: " + err.Error())
	}
	err = os.WriteFile("switch.json", updatedData, 0644)
	if err != nil {
		return c.SendString("Error writing to the file: " + err.Error())
	}

	// 성공적으로 파일을 업데이트했음을 클라이언트에게 알립니다.
	return c.SendString("Switches updated successfully")
	// Test
	// curl -X POST -H "Content-Type: application/json" -d '{"Car horn": false, "Infant Crying": false, "Glass": false, "Fire alarm": true, "Gunshot": true, "Bicycle Bell": true}' http://localhost:3000/switch
}

// Http polling with application
func Polling(c *fiber.Ctx) error {
	logger.MyLogger.Printf("request from", c.IP())

	// result.json 파일 읽기
	resultData, err := os.ReadFile("result.json")
	if err != nil {
		// 파일 읽기 오류 처리
		logger.MyLogger.Printf("Error reading result.json: %v", err)
		return c.SendString("Internal Server Error")
	}

	// decibel.json 파일 읽기
	decibelData, err := os.ReadFile("decibel.json")
	if err != nil {
		logger.MyLogger.Printf("Error reading decibel.json: %v", err)
		return c.SendString("Internal Server Error")
	}

	// JSON 파싱
	var result map[string]interface{}
	if err := json.Unmarshal(resultData, &result); err != nil {
		logger.MyLogger.Printf("Error parsing result.json: %v", err)
		return c.SendString("Error parsing result file")
	}

	var decibels map[string]interface{}
	if err := json.Unmarshal(decibelData, &decibels); err != nil {
		logger.MyLogger.Printf("Error parsing decibel.json: %v", err)
		return c.SendString("Error parsing decibel file")
	}

	// 두 JSON 객체 합치기
	for key, value := range decibels {
		result[key] = value
	}

	// 합쳐진 JSON 객체를 응답으로 보내기
	combinedData, err := json.Marshal(result)
	if err != nil {
		logger.MyLogger.Printf("Error marshaling combined data: %v", err)
		return c.SendString("Error marshaling combined data")
	}

	return c.Send(combinedData)
}

func SendDecibel(c *fiber.Ctx) error {
	logger.MyLogger.Printf("request from", c.IP())

	// Step 1: Parse the incoming JSON data
	var decibelData struct {
		Decibels float64 `json:"decibels"`
	}

	if err := c.BodyParser(&decibelData); err != nil {
		logger.MyLogger.Printf("Error parsing decibel data: %v", err)
		return c.SendString("Cannot parse decibel data")
	}

	// Step 2: Create a new JSON object with the decibel data
	decibelJSON := map[string]float64{
		"decibels": decibelData.Decibels,
	}

	// Step 3: Write the new decibel data to decibel.json
	updatedDecibelData, err := json.Marshal(decibelJSON)
	if err != nil {
		logger.MyLogger.Printf("Error marshaling decibel data: %v", err)
		return c.SendString("Error updating decibel file")
	}
	if err := os.WriteFile("decibel.json", updatedDecibelData, 0644); err != nil {
		logger.MyLogger.Printf("Error writing to decibel.json: %v", err)
		return c.SendString("Error writing to decibel file")
	}

	return c.SendStatus(fiber.StatusOK)
}
