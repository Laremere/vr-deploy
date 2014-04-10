package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

const clientFolder string = "deployFiles"

func main() {
	conn, err := net.Dial("tcp", os.Args[1]+":"+os.Args[2])
	if err != nil {
		log.Fatal(err)
	}

	_, err = conn.Write([]byte(os.Args[3] + "\n"))
	if err != nil {
		log.Fatal(err)
	}

	refreshFiles(os.Args[1] + ":2668/" + os.Args[3])

	buffer := bufio.NewReader(conn)

	var process *exec.Cmd

	for {
		command, err := buffer.ReadString('\n')
		if err != nil {
			log.Fatal(err)
		}
		command = command[0 : len(command)-1]

		switch command {
		case "refresh":
			log.Println("Refreshing file cache")
			refreshFiles(os.Args[1] + ":2668/" + os.Args[3])
		case "start":
			if process != nil {
				err = process.Process.Kill()
				process = nil
			}
			fileLocation, err := filepath.Abs(filepath.Join(clientFolder, "example.bat"))
			if err != nil {
				log.Fatal(err)
			}
			process = exec.Command(fileLocation)
			//process = exec.Command("example.bat")
			//process.Dir, err = filepath.Abs(clientFolder)
			process.Dir = clientFolder
			log.Println(process.Dir)
			if err != nil {
				log.Fatal(err)
			}
			err = process.Start()
			if err != nil {
				log.Fatal(err)
			}
		case "stop":
			if process != nil {
				err = process.Process.Kill()
				process = nil
			}
		default:
			log.Println("unkown command:", "'"+command+"'")
		}

	}
}

func refreshFiles(baseAddress string) {
	finished := false
	for !finished {
		finished = true
		resp, err := http.Get("http://" + baseAddress + "/hash/")
		if err != nil {
			log.Fatal(err)
		}
		defer resp.Body.Close()

		goal := make(map[string]string)
		buffer := bufio.NewReader(resp.Body)
		for err != io.EOF {
			line, err := buffer.ReadString('\n')
			log.Println("Heyo: ", line)
			if err != nil && err != io.EOF {
				log.Fatal(err)
			}
			if line == "" && err == io.EOF {
				break
			}
			components := strings.Split(line[0:len(line)-1], ":")
			goal[components[0]] = components[1]
		}
		curDirectory, err := filepath.Abs(clientFolder)
		if err != nil {
			log.Fatal(err)
		}
		current := scanFiles(curDirectory)
		log.Println("goal:", goal)
		log.Println("current:", current)

		for fileName := range goal {
			if goal[fileName] != current[fileName] {
				finished = false
				log.Println("Downloading", fileName)
				err := os.MkdirAll(filepath.Dir(path.Join(clientFolder, fileName)),
					os.ModePerm|os.ModeDir)
				if err != nil {
					log.Fatal(err)
				}

				file, err := os.Create(path.Join(clientFolder, fileName))
				if err != nil {
					log.Fatal(err)
				}
				defer file.Close()

				resp, err := http.Get("http://" + baseAddress + "/files/" + fileName)
				if err != nil {
					log.Fatal(err)
				}
				defer resp.Body.Close()

				_, err = io.Copy(file, resp.Body)
				if err != nil {
					log.Fatal(err)
				}
			}
		}

		for fileName := range current {
			_, in := goal[fileName]
			if !in {
				os.Remove(path.Join(clientFolder, fileName))
			}
		}
	}
}

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
