package main

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const (
	addr = ":3000"
	dbFile = "chat.db"
	timeLayout = "15:04"
)

type User struct {
	ID        uint `gorm:"primaryKey"`
	Name      string `gorm:"uniqueIndex"`
	Password  string
	CreatedAt time.Time
}

type Message struct {
	ID        uint `gorm:"primaryKey"`
	UserID    *uint
	UserName  string
	Addr      string
	Text      string
	CreatedAt time.Time
}

var (
	db *gorm.DB
	historyMutex sync.Mutex
	// in-memory history cached for quick send on connect (keeps last N or all)
)

func main() {
	var err error
	fmt.Println("Starting server on", addr)

	db, err = gorm.Open(sqlite.Open(dbFile), &gorm.Config{})
	if err != nil {
		fmt.Println("failed to open database:", err)
		os.Exit(1)
	}

	if err := db.AutoMigrate(&User{}, &Message{}); err != nil {
		fmt.Println("migrate error:", err)
		os.Exit(1)
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Println("listen error:", err)
		os.Exit(1)
	}
	defer ln.Close()

	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println("accept error:", err)
			continue
		}
		go handleConn(conn)
	}
}

func handleConn(conn net.Conn) {
	defer conn.Close()
	remote := conn.RemoteAddr().String()
	now := time.Now().Format(timeLayout)
	fmt.Printf("%s Client connected from %s\n", now, remote)

	// Send history to new client (from DB)
	if err := sendHistory(conn); err != nil {
		fmt.Println("error sending history:", err)
	}

	reader := bufio.NewReader(conn)

	// track current user if connected
	var currentUser *User

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Printf("%s Client %s disconnected\n", time.Now().Format(timeLayout), remote)
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Handle commands starting with /
		if strings.HasPrefix(line, "/") {
			resp, err := handleCommand(line, &currentUser, remote)
			if err != nil {
				conn.Write([]byte("ERR: " + err.Error() + "\n"))
			} else if resp != "" {
				conn.Write([]byte(resp + "\n"))
			}
			continue
		}

		// Not a command - process parsing tasks and store message
		// Save message to DB
		msg := Message{
			Text: line,
			Addr: remote,
			CreatedAt: time.Now(),
		}
		if currentUser != nil {
			msg.UserID = &currentUser.ID
			msg.UserName = currentUser.Name
		}
		if err := db.Create(&msg).Error; err != nil {
			fmt.Println("db create message error:", err)
		}

		// Log
		nameOrAddr := remote
		if currentUser != nil {
			nameOrAddr = currentUser.Name
		}
		logEntry := fmt.Sprintf("%s %s %s", time.Now().Format(timeLayout), nameOrAddr, line)
		fmt.Println(logEntry)

		// Respond according to parsing rules:
		// If message starts with "echo/add/mul" treat specially (as per 4.3.3)
		tokens := strings.Fields(line)
		var reply string
		switch tokens[0] {
		case "echo":
			if len(tokens) >= 2 {
				reply = strings.Join(tokens[1:], " ")
			} else {
				reply = ""
			}
		case "add":
			if len(tokens) == 3 {
				a, b := tokens[1], tokens[2]
				var ai, bi int
				_, err1 := fmt.Sscanf(a, "%d", &ai)
				_, err2 := fmt.Sscanf(b, "%d", &bi)
				if err1==nil && err2==nil {
					reply = fmt.Sprintf("%d", ai+bi)
				} else {
					reply = "ERR: add expects two integers"
				}
			} else {
				reply = "ERR: add expects two arguments"
			}
		case "mul":
			if len(tokens) == 3 {
				a, b := tokens[1], tokens[2]
				var ai, bi int
				_, err1 := fmt.Sscanf(a, "%d", &ai)
				_, err2 := fmt.Sscanf(b, "%d", &bi)
				if err1==nil && err2==nil {
					reply = fmt.Sprintf("%d", ai*bi)
				} else {
					reply = "ERR: mul expects two integers"
				}
			} else {
				reply = "ERR: mul expects two arguments"
			}
		default:
			// other parsing commands from 4.3
			// bytes -> number of bytes in message
			// words -> number of words
			// fallback: echo back original + " from server"
			if strings.HasPrefix(line, "bytes ") {
				rest := strings.TrimPrefix(line, "bytes ")
				reply = fmt.Sprintf("%d", len([]byte(rest)))
			} else if strings.HasPrefix(line, "words ") {
				rest := strings.TrimPrefix(line, "words ")
				reply = fmt.Sprintf("%d", len(strings.Fields(rest)))
			} else {
				reply = line + " from server"
			}
		}

		// send reply
		_, err = conn.Write([]byte(reply + "\n"))
		if err != nil {
			fmt.Println("write error:", err)
			return
		}
	}
}

func sendHistory(conn net.Conn) error {
	var msgs []Message
	if err := db.Order("created_at asc").Find(&msgs).Error; err != nil {
		return err
	}
	for _, m := range msgs {
		displayName := m.Addr
		if m.UserName != "" {
			displayName = m.UserName
		}
		line := fmt.Sprintf("%s %s %s\n", m.CreatedAt.Format(timeLayout), displayName, m.Text)
		if _, err := conn.Write([]byte(line)); err != nil {
			return err
		}
	}
	return nil
}

func handleCommand(line string, currentUser **User, remote string) (string, error) {
	parts := strings.Fields(line)
	cmd := strings.TrimPrefix(parts[0], "/")
	switch cmd {
	case "setname":
		// /setname name password
		if len(parts) != 3 {
			return "", errors.New("usage: /setname <name> <password>")
		}
		name := parts[1]
		pw := parts[2]
		// check unique
		var exists User
		if err := db.Where("name = ?", name).First(&exists).Error; err == nil {
			return "", errors.New("name already taken")
		}
		// hash password
		h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
		if err != nil {
			return "", err
		}
		u := User{Name: name, Password: string(h), CreatedAt: time.Now()}
		if err := db.Create(&u).Error; err != nil {
			return "", err
		}
		*currentUser = &u
		return "OK: registered and logged in as " + name, nil
	case "connect":
		// /connect name password
		if len(parts) != 3 {
			return "", errors.New("usage: /connect <name> <password>")
		}
		name := parts[1]
		pw := parts[2]
		var u User
		if err := db.Where("name = ?", name).First(&u).Error; err != nil {
			return "", errors.New("no such user")
		}
		if err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(pw)); err != nil {
			return "", errors.New("invalid password")
		}
		*currentUser = &u
		return "OK: logged in as " + name, nil
	case "list":
		// list all users except current
		var users []User
		if err := db.Find(&users).Error; err != nil {
			return "", err
		}
		var out []string
		for _, u := range users {
			if *currentUser != nil && u.ID == (*currentUser).ID {
				continue
			}
			out = append(out, u.Name)
		}
		if len(out) == 0 {
			return "(no other users)", nil
		}
		return strings.Join(out, ", "), nil
	default:
		return "", errors.New("unknown command: " + cmd)
	}
}
