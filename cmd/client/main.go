package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

const serverAddr = "127.0.0.1:3000"

func main() {
	// allow optional single message argument
	var initialMsg string
	if len(os.Args) >= 2 {
		initialMsg = strings.Join(os.Args[1:], " ")
	}

	fmt.Printf("Trying to connect to %s...\n", serverAddr)
	conn, err := net.DialTimeout("tcp", serverAddr, 5*time.Second)
	if err != nil {
		fmt.Println("Connection error:", err)
		return
	}
	defer conn.Close()
	fmt.Println("Connected!")

	// goroutine to read server messages continuously
	go func() {
		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			fmt.Println("Received:", scanner.Text())
		}
		fmt.Println("Server connection closed.")
		os.Exit(0)
	}()

	// if initialMsg provided, send it once
	if initialMsg != "" {
		sendLine(conn, initialMsg)
	}

	// interactive loop
	stdin := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("> ")
		line, err := stdin.ReadString('\n')
		if err != nil {
			fmt.Println("input error:", err)
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == "exit" {
			fmt.Println("Exiting.")
			return
		}
		sendLine(conn, line)
	}
}

func sendLine(conn net.Conn, s string) {
	_, err := conn.Write([]byte(s + "\n"))
	if err != nil {
		fmt.Println("send error:", err)
	}
}
