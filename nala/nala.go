package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"

	"github.com/gin-gonic/gin"
)

// ! Delete later
type Message struct {
	Msg string `json:"msg"`
}

// TODO Create a trigger endpoint
func triggerDetection(ctx *gin.Context) {
	var message = Message{Msg: "Detection triggered"}
	ctx.IndentedJSON(http.StatusOK, message)
}

// Runs "testyp.py" and prints the output
func pyCall() {
	//Sets Arguments to the command
	program := "python"
	scriptLocation := "./testpy.py"
	msg := "Hello Python"
	csvLocation := "..//dataset/system-1.csv"

	cmd := exec.Command(program, scriptLocation, msg, csvLocation)
	//Debug prints Stderr error
	cmd.Stderr = os.Stderr
	//executes command, listends to stdout, puts w/e into "out" var unless error
	out, err := cmd.Output()
	if err != nil {
		log.Fatal(err) // Only gives exit 1 if error, use "cmd.Stderr = os.Stderr" (import os)
	}
	//Print, Need explicit typing or it prints an array with unicode numbers
	fmt.Println(string(out))
}

func main() {
	pyCall()
	router := gin.Default()
	router.GET("/nala/trigger", triggerDetection)
	router.Run("localhost:8088")
}
