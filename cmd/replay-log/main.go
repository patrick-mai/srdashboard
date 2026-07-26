// replay-log reads a DISAG JSON log file and sends each Shot message via UDP
// to the viewer at 127.0.0.1:port, with a configurable interval between shots.
// The log file is decoded from Windows-1252 (CP1252) to UTF-8 so German umlauts (äöü) display correctly.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"io"
	"log"
	"net"
	"os"
	"time"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

func main() {
	logPath := flag.String("log", "", "Path to OUT-JSONInterface.log.txt (required)")
	port := flag.Int("port", 30169, "UDP port of the viewer")
	interval := flag.Duration("interval", 300*time.Millisecond, "Delay between shots (e.g. 300ms)")
	encoding := flag.String("encoding", "cp1252", "Log file encoding: cp1252 (Windows, default) or utf8")
	flag.Parse()

	if *logPath == "" {
		log.Fatal("usage: go run . -log <path-to-log> [-port 30169] [-interval 300ms] [-encoding cp1252|utf8]")
	}

	f, err := os.Open(*logPath)
	if err != nil {
		log.Fatalf("open log: %v", err)
	}
	defer f.Close()

	var logReader io.Reader = f
	if *encoding == "cp1252" {
		// Decode Windows-1252 to UTF-8 so German umlauts (ö, ä, ü) display correctly
		logReader = transform.NewReader(f, charmap.Windows1252.NewDecoder())
	}

	conn, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: *port})
	if err != nil {
		log.Fatalf("dial UDP: %v", err)
	}
	defer conn.Close()

	log.Printf("sending shots from %s to 127.0.0.1:%d every %v", *logPath, *port, *interval)

	scanner := bufio.NewScanner(logReader)
	var sent int
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var envelope struct {
			MessageType string `json:"MessageType"`
			MessageVerb string `json:"MessageVerb"`
		}
		if err := json.Unmarshal(line, &envelope); err != nil {
			continue
		}
		if envelope.MessageType != "Event" || envelope.MessageVerb != "Shot" {
			continue
		}
		if _, err := conn.Write(line); err != nil {
			log.Printf("write: %v", err)
			continue
		}
		sent++
		log.Printf("shot %d sent (range from payload)", sent)
		time.Sleep(*interval)
	}
	if err := scanner.Err(); err != nil {
		log.Fatalf("read log: %v", err)
	}
	log.Printf("done: %d shots sent", sent)
}
