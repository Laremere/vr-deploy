package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	log.Println("Reading config")

	config, err := readConfigs()
	if err != nil {
		log.Fatal(err)
	}
	log.Println(config)

	log.Println("Starting server")
	ln, err := net.Listen("tcp", ":"+config.ListenPort)
	if err != nil {
		log.Fatal(err)
	}

	for clientName, client := range config.Clients {
		http.Handle("/"+clientName+"/files/",
			http.StripPrefix("/"+clientName+"/files/",
				http.FileServer(http.Dir(client.Source))))
		http.HandleFunc("/"+clientName+"/hash/",
			func(w http.ResponseWriter, r *http.Request) {
				for fileName, hash := range scanFiles(client.Source) {
					_, err := fmt.Fprintf(w, "%s:%s\n", fileName, hash)
					if err != nil {
						log.Println("Err servering http:", err)
						return
					}
				}
			})
	}

	go http.ListenAndServe(":"+config.HttpPort, nil)

	ccChan := make(chan *ConnChannels)
	ccCloseChan := make(chan *ConnChannels)

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				fmt.Println(err)
				continue
			}
			log.Println("connected", conn.LocalAddr())
			go handleConnection(conn, ccChan, ccCloseChan)
		}
	}()

	commands := make(chan string)

	go func() {
		for {
			command, err := bufio.NewReader(os.Stdin).ReadString('\n')
			if err != nil {
				log.Fatal("Error reading io in: ", err)
			}
			commands <- strings.Trim(command, "\r\n")
		}
	}()

	clients := make([]*ConnChannels, 0)

	for {
		select {
		case command := <-commands:
			for _, client := range clients {
				client.commands <- command
			}
		case cc := <-ccChan:
			clients = append(clients, cc)
		case cc := <-ccCloseChan:
			for i := 0; i < len(clients); i++ {
				if clients[i] == cc {
					clients[i] = clients[len(clients)-1]
					clients = clients[0 : len(clients)-1]
					break
				}
			}
		}
	}
}

////////////////////////////////////////////////////////////

type ConnChannels struct {
	name     string
	commands chan string
}

func handleConnection(conn net.Conn, ccChan chan *ConnChannels,
	ccCloseChan chan *ConnChannels) {

	defer conn.Close()
	name, err := bufio.NewReader(conn).ReadString('\n')
	name = name[0 : len(name)-1] //Remove trailing newline
	if err != nil {
		log.Println("Dropping client for error: ", err)
		err = conn.Close()
		if err != nil {
			log.Println("Additional disconnect error: ", err)
		}
		return
	}
	log.Println("connected client version: ", name)

	cc := ConnChannels{name, make(chan string)}
	ccChan <- &cc

	for {
		command := <-cc.commands
		_, err = fmt.Fprint(conn, command, "\n")
		if err != nil {
			log.Println("Dropping client for error: ", err)
			err = conn.Close()
			if err != nil {
				log.Println("Additional disconnect error: ", err)
			}
			break
		}
	}

	for {
		select {
		case <-cc.commands:
		case ccCloseChan <- &cc:
			return
		}
	}
}

////////////////////////////////////////////////////////////

func readConfigs() (*Config, error) {
	file, err := ioutil.ReadFile("config.json")
	if err != nil {
		return nil, err
	}

	var config Config
	err = json.Unmarshal(file, &config)
	return &config, err
}

type Config struct {
	ListenPort string
	HttpPort   string
	Clients    map[string]ClientConfig
}

type ClientConfig struct {
	Source string
	Run    string
}

////////////////////////////////////////////////////////////

func scanFiles(basePath string) map[string]string {
	result := make(map[string]string)

	var processFile = func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		httpPath, err := filepath.Rel(basePath, path)
		if err != nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		hash := sha1.New()
		_, err = io.Copy(hash, file)
		if err != nil {
			return err
		}
		result[filepath.ToSlash(httpPath)] = hex.EncodeToString(hash.Sum(nil))
		return nil
	}

	filepath.Walk(basePath, processFile)
	return result
}
