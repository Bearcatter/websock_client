package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"time"

	"github.com/gorilla/websocket"
)

const (
	RTSP_PLAYER = "vlc"
)

var (
	addr       = flag.String("addr", "localhost:8080", "http service address")
	rtsp       = flag.String("rtsp", "rtsp://192.168.1.26:554/au:scanner.au", "rtsp client ")
	rtspListen = flag.Bool("audio", false, "stream rtsp audio")
)

func main() {

	flag.Parse()
	log.SetFlags(0)

	if *rtspListen {
		if openRtspFound() {
			err := startRtspLisener()
			if err != nil {
				fmt.Printf("failed to start listener")
			}
		} else {
			log.Fatal("Can't listen to RTSP stream, openrtsp not installed or on PATH\n")
		}
	}

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	u := url.URL{Scheme: "ws", Host: *addr, Path: "/radio"}
	log.Printf("connecting to %s\n", u.String())

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close()
	log.Printf("connected\n")
	// DOESN'T SEEM TO WORK AS EXPECTED c.SetReadDeadline(time.Now().Add(2 * time.Second))

	done := make(chan bool)
	done_input := make(chan bool)

	// start input prompt routine
	go user_input(c, done_input)

	// start WS read message routine
	go func() {

		for {
			select {
			case <-done:
				return
			case <-done_input:
				return
			default:
				_, message, err := c.ReadMessage()
				if err != nil {
					if e, ok := err.(net.Error); !ok || !e.Timeout() {
						log.Printf("error reading: %s\n", e)
						return
					} else {
						// timedout
						time.Sleep(time.Millisecond * 50)
						continue
					}
				}
				log.Printf("recv: %s", message)
			}
		}
	}()

	// wait for interrupt or done
	wait(c, done, done_input, interrupt)

	log.Printf("shutting down WS Socket and terminating\n")
	err = c.WriteMessage(websocket.TextMessage, []byte(formatted("quit")))

	// Cleanly close the connection by sending a close message and then
	// waiting (with timeout) for the server to close the connection.
	err = c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	if err != nil {
		log.Println("write close:", err)
		return
	}
}

func wait(c *websocket.Conn, done, done_input chan bool, interrupt chan os.Signal) {
	for {
		select {
		case <-done_input:
			close(done)
			return
		case <-done:
			close(done_input)
			return
		case <-interrupt:
			log.Println("received interrupt")
			close(done)
			close(done_input)
			return
		default:
			time.Sleep(time.Millisecond * 50)
		}
	}
}

func user_input(c *websocket.Conn, done chan bool) {

	scanner := bufio.NewScanner(os.Stdin)
	fmt.Printf("> ")
	for scanner.Scan() {

		message := scanner.Text()
		if len(message) < 1 {
			fmt.Printf("> ")
			continue
		}

		// See if we quit
		if message == "quit" {
			done <- true
			return
		}
		err := c.WriteMessage(websocket.TextMessage, []byte(formatted(message)))
		if err != nil {
			log.Println("write:", err)
			return
		} else {
			log.Printf("wrote: %s\n", message)
		}
		fmt.Printf("> ")
	}
	if err := scanner.Err(); err != nil {
		log.Println(err)
		return
	}
}

func formatted(msg string) []byte {
	return []byte(msg + "\r")
}

func startRtspLisener() error {

	fmt.Printf("starting rtsp listener\n")
	//cmd := exec.Command("openrtsp", "rtsp://192.168.1.26:554/au:scanner.au")
	cmd := exec.Command(RTSP_PLAYER, "-I", "rc", "rtsp://192.168.1.26:554/au:scanner.au")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatal(err)
		return err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Fatal(err)
		return err
	}

	if err := cmd.Start(); err != nil {
		log.Fatal(err)
		return err
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {

		message := scanner.Text()
		log.Printf("stdout: %s\n", message)
	}
	scanner = bufio.NewScanner(stderr)
	for scanner.Scan() {

		message := scanner.Text()
		log.Printf("stderr: %s\n", message)
	}

	if err := cmd.Wait(); err != nil {
		log.Fatal(err)
		return err
	}
	return nil
}

func openRtspFound() bool {
	cmd := exec.Command("which", RTSP_PLAYER)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return false
	}
	if err := cmd.Start(); err != nil {
		return false
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {

		message := scanner.Text()
		log.Printf("%s\n", message)
	}

	if err := cmd.Wait(); err != nil {
		return false
	}
	return true
}
