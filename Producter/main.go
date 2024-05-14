package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sqweek/dialog"
)

const (
	queueName    = "audio_queue"
	baiduAPIURL  = "https://vop.baidu.com/pro_api"
	baiduToken   = "24.6a8fc2df2ab856c95b232f5d33da2508.2592000.1718204531.282335-70268624"
	baiduDevPID  = 80001
	baiduChannel = 1
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

type ResponseData struct {
	Result string `json:"result"`
	Hash   string `json:"hash"`
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
		"audio_queue", // name
		true,          // durable
		false,         // delete when unused
		false,         // exclusive
		false,         // no-wait
		nil,           // arguments
	)
	if err != nil {
		log.Fatalf("Failed to declare a queue: %v", err)
	}

	q2, err := ch.QueueDeclare(
		"response", // name
		true,       // durable
		false,      // delete when unused
		false,      // exclusive
		false,      // no-wait
		nil,        // arguments
	)
	if err != nil {
		log.Fatalf("Failed to declare a queue: %v", err)
	}

	msgs, err := ch.Consume(q2.Name, "", false, false, false, false, nil)
	if err != nil {
		log.Fatalf("Failed to register a consumer: %v", err)
	}

	for {
		filePath, err := dialog.File().Filter("WAV 文件", "wav").Load()
		if err != nil {
			fmt.Println("无法打开文件选择对话框:", err)
			return
		}

		// 读取所选择的 WAV 文件
		audioData, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Println("无法读取文件:", err)
			return
		}
		audioLen := len(audioData)
		audioLenStr := strconv.Itoa(audioLen)
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

		type AudioData struct {
			AudioLenStr string `json:"audioLenStr"`
			AudioBase64 string `json:"audioBase64"`
			Hash        string `json:"hash"`
		}

		audioInfo := AudioData{
			AudioLenStr: audioLenStr,
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
						fmt.Println("解析JSON数据失败:", err)
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
						fmt.Println("hash 校验失败")
						d.Nack(false, true) // 消息重新入队
					}
				case <-ctx.Done():
					return
				}
			}
		}(ctx, hash)

		time.Sleep(2 * time.Second)
		fmt.Print("是否继续上传文件？(输入 'yes' 继续，'no' 退出): ")
		var input string
		fmt.Scanln(&input)
		if input == "no" {
			break
		}
	}
}
