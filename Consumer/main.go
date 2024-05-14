package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	amqp "github.com/rabbitmq/amqp091-go"
	"log"
	"net/http"
	"strconv"
	"time"
)

const (
	queueName    = "audio_queue"
	baiduAPIURL  = "https://vop.baidu.com/pro_api"
	baiduToken   = "24.6a8fc2df2ab856c95b232f5d33da2508.2592000.1718204531.282335-70268624"
	baiduDevPID  = 80001
	baiduChannel = 1
)

type AudioUploadRequest struct {
	Format  string `json:"format"`
	Rate    int    `json:"rate"`
	Channel int    `json:"channel"`
	Cuid    string `json:"cuid"`
	Token   string `json:"token"`
	DevPid  int    `json:"dev_pid"`
	Speech  string `json:"speech"`
	Len     int    `json:"len"`
}

type AudioUploadResponse struct {
	CorpusNo string   `json:"corpus_no"`
	ErrMsg   string   `json:"err_msg"`
	ErrNo    int      `json:"err_no"`
	Result   []string `json:"result"`
	Sn       string   `json:"sn"`
}

type AudioData struct {
	AudioLenStr string `json:"audioLenStr"`
	AudioBase64 string `json:"audioBase64"`
	Hash        string `json:"hash"`
}

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

	// Declare the queue
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

	q2, err2 := ch.QueueDeclare(
		"response", // name
		true,       // durable
		false,      // delete when unused
		false,      // exclusive
		false,      // no-wait
		nil,        // arguments
	)
	if err2 != nil {
		log.Fatalf("Failed to declare a queue: %v", err2)
	}

	msg, err := ch.Consume(q.Name, "", true, false, false, false, nil)
	if err != nil {
		log.Fatalf("Failed to register a consumer: %v", err)
	}

	var forever chan struct{}
	go func() {
		for d := range msg {
			// d.Body是一个字符串
			audioData := d.Body
			// 解析JSON数据
			var audio AudioData
			err := json.Unmarshal(audioData, &audio)
			if err != nil {
				fmt.Println("解析JSON数据失败:", err)
				return
			}
			fmt.Print("音频数据:", audio, "\n")
			// 从JSON数据中获取音频数据
			audioLenStr := audio.AudioLenStr
			audioBase64 := audio.AudioBase64
			hash := audio.Hash
			fmt.Print("音频数据哈希值:", hash, "\n")
			//将audioLenStr转换为int
			audioLen, err := strconv.Atoi(audioLenStr)
			if err != nil {
				fmt.Println("转换音频数据长度失败:", err)
				return
			}

			requestData := AudioUploadRequest{
				Format:  "wav",
				Rate:    16000,
				Channel: baiduChannel,
				Cuid:    "learn",
				Token:   baiduToken,
				DevPid:  baiduDevPID,
				Speech:  audioBase64,
				Len:     audioLen,
			}
			requestJSON, err := json.Marshal(requestData)
			if err != nil {
				fmt.Println("JSON编码失败:", err)
				return
			}
			resp, err := http.Post(baiduAPIURL, "application/json", bytes.NewBuffer(requestJSON))
			if err != nil {
				fmt.Println("发送POST请求失败:", err)
				return
			}
			defer resp.Body.Close()

			// 解析响应数据
			var response AudioUploadResponse
			err = json.NewDecoder(resp.Body).Decode(&response)
			if err != nil {
				fmt.Println("解析响应数据失败:", err)
				return
			}

			// 打印响应结果
			fmt.Println("语音识别结果:", response.Result)
			resultStr := response.Result[0]
			//response.Result[0]转换成二进制
			responseData := ResponseData{
				Result: resultStr,
				Hash:   hash,
			}
			jsonResponseData, err := json.Marshal(responseData)
			// 发送响应数据
			ch.PublishWithContext(context.Background(), "", q2.Name, false, false, amqp.Publishing{
				ContentType: "application/json",
				Body:        jsonResponseData,
			})
			//等待一秒
			time.Sleep(time.Second)
		}
	}()
	<-forever

}
