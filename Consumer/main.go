package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	amqp "github.com/rabbitmq/amqp091-go"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

const (
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
	AudioLen    int    `json:"audioLen"`
	AudioBase64 string `json:"audioBase64"`
	Hash        string `json:"hash"`
}

type ResponseData struct {
	Result string `json:"result"`
	Hash   string `json:"hash"`
}

type TextData struct {
	Text string `json:"text"`
	Hash string `json:"hash"`
}

type TextResponseData struct {
	AudioData string `json:"audioData"`
	Hash      string `json:"hash"`
}

type PostData struct {
	Text          string `json:"text"`
	Text_language string `json:"text_language"`
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
	if err != nil {
		log.Fatalf("Failed to declare a queue: %v", err)
	}

	q4, err := ch.QueueDeclare(
		"text_to_audio_response", // name
		true,                     // durable
		false,                    // delete when unused
		false,                    // exclusive
		false,                    // no-wait
		nil,                      // arguments
	)
	if err != nil {
		log.Fatalf("Failed to declare a queue: %v", err)
	}

	// Consume messages from the queues
	msg, err := ch.Consume(q.Name, "", true, false, false, false, nil)
	if err != nil {
		log.Fatalf("Failed to register a consumer: %v", err)
	}

	msg2, err := ch.Consume(q3.Name, "", true, false, false, false, nil)
	if err != nil {
		log.Fatalf("Failed to register a consumer: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(2) // 设置等待两个协程

	go func() {
		defer wg.Done() // 协程完成后调用Done

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
			// 从JSON数据中获取音频数据
			audioLen := audio.AudioLen
			fmt.Println("音频数据长度:", audioLen)
			audioBase64 := audio.AudioBase64
			hash := audio.Hash
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
			responseData := ResponseData{
				Result: resultStr,
				Hash:   hash,
			}
			jsonResponseData, err := json.Marshal(responseData)
			if err != nil {
				fmt.Println("JSON编码失败:", err)
				return
			}
			// 发送响应数据
			ch.PublishWithContext(context.Background(), "", q2.Name, false, false, amqp.Publishing{
				ContentType: "application/json",
				Body:        jsonResponseData,
			})
			//等待一秒
			time.Sleep(time.Second)
		}
	}()

	go func() {
		defer wg.Done() // 协程完成后调用Done

		for d := range msg2 {
			textData := d.Body
			// 解析JSON数据
			var text TextData
			err := json.Unmarshal(textData, &text)
			if err != nil {
				fmt.Println("解析JSON数据失败:", err)
				return
			}
			// 从JSON数据中获取文本数据
			textStr := text.Text
			hash := text.Hash
			fmt.Println("哈希值:", hash)
			//发送POST请求
			postData := PostData{
				Text:          textStr,
				Text_language: "zh",
			}
			fmt.Println("发送POST请求数据:", postData)
			postJSON, err := json.Marshal(postData)
			if err != nil {
				fmt.Println("JSON编码失败:", err)
				return
			}

			resp, err := http.Post("http://localhost:9880", "application/json", bytes.NewBuffer(postJSON))
			if err != nil {
				fmt.Println("发送POST请求失败:", err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				fmt.Println("服务器返回错误状态码:", resp.StatusCode)
				return
			}

			// 读取响应体
			var buf bytes.Buffer
			_, err = io.Copy(&buf, resp.Body)
			if err != nil {
				fmt.Println("读取响应数据失败:", err)
				return
			}

			// Base64编码
			audioData := base64.StdEncoding.EncodeToString(buf.Bytes())

			// 封装数据
			textResponseData := TextResponseData{
				AudioData: audioData,
				Hash:      hash, // 替换为实际hash
			}

			// JSON编码
			jsonTextResponseData, err := json.Marshal(textResponseData)
			if err != nil {
				fmt.Println("JSON编码失败:", err)
				return
			}

			// 发送响应数据
			ch.PublishWithContext(context.Background(), "", q4.Name, false, false, amqp.Publishing{
				ContentType: "application/json",
				Body:        jsonTextResponseData,
			})
			//等待一秒
			time.Sleep(time.Second)
		}
	}()

	wg.Wait() // 等待两个协程都完成
}
