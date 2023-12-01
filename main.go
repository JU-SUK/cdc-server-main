package main

import (
	"context"
	"fmt"
	logger "solution/logger"
	router "solution/router"

	fiber "github.com/gofiber/fiber/v2"
)

var ctx = context.Background()

func main() {
	app := fiber.New(fiber.Config{AppName: "Hearsitter!"})

	app.Get("/", router.HelloWorld)    //route for check server on
	app.Get("/ping", router.PingPong)  //route for check ml server on
	app.Post("/uint", router.MlServer) //route for send byte[] to ml server
	app.Post("/file", router.Files)    //route for send file to ml server
	app.Post("/switch", router.UpdateSwitch)
	app.Get("/history/latest", router.Polling)
	app.Post("/send_decibel", router.SendDecibel)
	logger.Start()
	fmt.Println("HI!")
	app.Listen(":3000")
}
