package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sqweek/dialog"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

type ResponseData struct {
	Result string `json:"result"`
	Hash   string `json:"hash"`
}

type AudioData struct {
	AudioLen    int    `json:"audioLen"`
	AudioBase64 string `json:"audioBase64"`
	Hash        string `json:"hash"`
}

type TextData struct {
	Text string `json:"text"`
	Hash string `json:"hash"`
}

type TextResponseData struct {
	AudioData string `json:"audioData"`
	Hash      string `json:"hash"`
}

func main() {
	// Connect to RabbitMQ
	conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	if err != nil {
		log.Fatalf("Failed to connect to RabbitMQ: %v", err)
	}
	defer conn.Close()

	// Open a channel
	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("Failed to open a channel: %v", err)
	}
	defer ch.Close()

	// Declare the queues
	q, err := ch.QueueDeclare(
		"audio_to_text_request", // name
		true,                    // durable
		false,                   // delete when unused
		false,                   // exclusive
		false,                   // no-wait
		nil,                     // arguments
	)
	if err != nil {
		log.Fatalf("Failed to declare a queue: %v", err)
	}

	q2, err := ch.QueueDeclare(
		"audio_to_text_response", // name
		true,                     // durable
		false,                    // delete when unused
		false,                    // exclusive
		false,                    // no-wait
		nil,                      // arguments
	)
	if err != nil {
		log.Fatalf("Failed to declare a queue: %v", err)
	}

	q3, err := ch.QueueDeclare(
		"text_to_audio_request", // name
		true,                    // durable
		false,                   // delete when unused
		false,                   // exclusive
		false,                   // no-wait
		nil,                     // arguments
	)

	q4, err := ch.QueueDeclare(
		"text_to_audio_response", // name
		true,                     // durable
		false,                    // delete when unused
		false,                    // exclusive
		false,                    // no-wait
		nil,                      // arguments
	)

	msgs, err := ch.Consume(q2.Name, "", false, false, false, false, nil)
	msgs2, err := ch.Consume(q4.Name, "", false, false, false, false, nil)

	if err != nil {
		log.Fatalf("Failed to register a consumer: %v", err)
	}

	for {
		fmt.Print("请选择功能（1: 音频转文字, 2: 文字转音频, no: 退出）: ")
		var choice string
		fmt.Scanln(&choice)

		switch choice {
		case "1":
			filePath, err := dialog.File().Filter("WAV 文件", "wav").Load()
			if err != nil {
				fmt.Println("无法打开文件选择对话框:", err)
				continue
			}

			// 读取所选择的 WAV 文件
			audioData, err := os.ReadFile(filePath)
			if err != nil {
				fmt.Println("无法读取文件:", err)
				continue
			}
			audioLen := len(audioData)
			audioBase64 := base64.StdEncoding.EncodeToString(audioData)

			randStringBytes := func(n int) string {
				b := make([]byte, n)
				for i := range b {
					b[i] = letterBytes[rand.Intn(len(letterBytes))]
				}
				return string(b)
			}

			// 设置随机数种子
			rand.Seed(time.Now().UnixNano())

			// 生成64个字母的随机字符串
			hash := randStringBytes(64)

			audioInfo := AudioData{
				AudioLen:    audioLen,
				AudioBase64: audioBase64,
				Hash:        hash,
			}

			jsonData, err := json.Marshal(audioInfo)
			if err != nil {
				log.Fatalf("Failed to marshal JSON: %v", err)
			}

			err = ch.PublishWithContext(context.Background(), "", q.Name, false, false, amqp.Publishing{
				ContentType: "application/json",
				Body:        jsonData,
			})
			if err != nil {
				log.Fatalf("Failed to publish a message: %v", err)
			}

			// 创建一个可以取消的上下文
			ctx, cancel := context.WithCancel(context.Background())

			go func(ctx context.Context, hash string) {
				for {
					select {
					case d := <-msgs:
						var jsonResponseData ResponseData
						err := json.Unmarshal(d.Body, &jsonResponseData)
						if err != nil {
							fmt.Println("解析JSON数据失败1:", err)
							continue
						}

						result := jsonResponseData.Result
						hashback := jsonResponseData.Hash

						if hashback == hash {
							fmt.Printf("语音识别结果: %s\n", result)
							d.Ack(false)
							cancel() // 关闭 goroutine
							return
						} else {
							d.Nack(false, true) // 消息重新入队
						}

					case <-ctx.Done():
						return
					}
				}
			}(ctx, hash)
			time.Sleep(2 * time.Second)

		case "2":
			// 文字转音频功能占位符
			// 请输入要转换成音频的文字
			fmt.Print("请输入要转换成音频的文字: ")
			var text string
			fmt.Scanln(&text)
			randStringBytes := func(n int) string {
				b := make([]byte, n)
				for i := range b {
					b[i] = letterBytes[rand.Intn(len(letterBytes))]
				}
				return string(b)
			}

			// 设置随机数种子
			rand.Seed(time.Now().UnixNano())

			// 生成64个字母的随机字符串
			hash := randStringBytes(64)

			textInfo := TextData{
				Text: text,
				Hash: hash,
			}

			jsonTextData, err := json.Marshal(textInfo)

			err = ch.PublishWithContext(context.Background(), "", q3.Name, false, false, amqp.Publishing{
				ContentType: "application/json",
				Body:        jsonTextData,
			})
			if err != nil {
				log.Fatalf("Failed to publish a message: %v", err)
			}
			ctx, cancel := context.WithCancel(context.Background())

			go func(ctx context.Context, hash string) {
				for {
					select {
					case d := <-msgs2:
						var textResponseData TextResponseData
						err := json.Unmarshal(d.Body, &textResponseData)
						if err != nil {
							fmt.Println("JSON解码失败:", err)
							continue
						}
						hashback := textResponseData.Hash
						audioData, err := base64.StdEncoding.DecodeString(textResponseData.AudioData)
						if err != nil {
							fmt.Println("Base64解码失败:", err)
							continue
						}

						if hashback == hash {
							outputFileName := hash + "_output.wav"
							// 保存WAV文件
							err = ioutil.WriteFile(outputFileName, audioData, 0644)
							if err != nil {
								fmt.Println("保存文件失败:", err)
								continue
							}

							fmt.Println("WAV文件保存成功！")
							fmt.Print("请选择功能（1: 音频转文字, 2: 文字转音频, no: 退出）: ")

							d.Ack(false)
							cancel() // 关闭 goroutine
							return
						} else {
							d.Nack(false, true) // 消息重新入队
						}

					case <-ctx.Done():
						return
					}
				}
			}(ctx, hash)

		case "no":
			fmt.Println("程序退出")
			return

		default:
			fmt.Println("无效的选择，请输入 '1', '2' 或 'no'")
		}
	}
}
