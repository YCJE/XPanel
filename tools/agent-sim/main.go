// agent-sim 模拟一个 gost 中转 agent，用于调试面板与节点之间的通信协议。
//
// 用法：
//
//	go run ./tools/agent-sim -addr 127.0.0.1:2053 -secret <节点Token>
//
// 行为：连接 ws://<addr>/system-info 注册上线；每 2 秒上报系统信息；
// 收到面板命令（AddService/AddChains/...）后打印并回执成功。
package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

type cryptoBox struct{ key []byte }

func newCryptoBox(secret string) *cryptoBox {
	h := sha256.Sum256([]byte(secret))
	return &cryptoBox{key: h[:]}
}

func (c *cryptoBox) decrypt(data string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, err
	}
	block, _ := aes.NewCipher(c.key)
	gcm, _ := cipher.NewGCM(block)
	n := gcm.NonceSize()
	return gcm.Open(nil, raw[:n], raw[n:], nil)
}

func (c *cryptoBox) encrypt(data []byte) (string, error) {
	block, _ := aes.NewCipher(c.key)
	gcm, _ := cipher.NewGCM(block)
	nonce := make([]byte, gcm.NonceSize())
	for i := range nonce {
		nonce[i] = byte(time.Now().UnixNano() >> (i % 8))
	}
	sealed := gcm.Seal(nonce, nonce, data, nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

type commandMessage struct {
	Type      string          `json:"type"`
	Data      json.RawMessage `json:"data"`
	RequestId string          `json:"requestId,omitempty"`
}

type commandResponse struct {
	Type      string `json:"type"`
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	RequestId string `json:"requestId,omitempty"`
}

func main() {
	addr := flag.String("addr", "127.0.0.1:2053", "面板地址:端口")
	secret := flag.String("secret", "", "节点 Token")
	flag.Parse()
	if *secret == "" {
		log.Fatal("缺少 -secret")
	}

	box := newCryptoBox(*secret)
	url := fmt.Sprintf("ws://%s/system-info?type=1&secret=%s&version=sim-1.0&http=0&tls=0&socks=0", *addr, *secret)
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		log.Fatalf("握手失败: %v", err)
	}
	log.Println("✅ 已连接面板:", url)
	defer conn.Close()

	done := make(chan struct{})
	// 心跳：每 2 秒上报系统信息
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				info, _ := json.Marshal(map[string]any{
					"uptime": 1234, "cpu_usage": 5.5, "memory_usage": 21.0,
					"bytes_received": 1024, "bytes_transmitted": 2048,
				})
				enc, _ := box.encrypt(info)
				wire, _ := json.Marshal(map[string]any{"encrypted": true, "data": enc, "timestamp": time.Now().Unix()})
				if err := conn.WriteMessage(websocket.TextMessage, wire); err != nil {
					log.Println("上报失败:", err)
					return
				}
			}
		}
	}()

	for {
		mt, raw, err := conn.ReadMessage()
		if err != nil {
			log.Println("连接断开:", err)
			close(done)
			return
		}
		if mt != websocket.TextMessage {
			continue
		}
		var wrapper struct {
			Encrypted bool   `json:"encrypted"`
			Data      string `json:"data"`
		}
		if json.Unmarshal(raw, &wrapper) == nil && wrapper.Encrypted {
			if raw, err = box.decrypt(wrapper.Data); err != nil {
				log.Println("解密失败:", err)
				continue
			}
		}
		if string(raw) == `{"type":"call"}` {
			continue // 心跳确认
		}
		var cmd commandMessage
		if err := json.Unmarshal(raw, &cmd); err != nil {
			log.Println("收到非命令消息:", string(raw))
			continue
		}
		log.Printf("🔔 收到命令 %s: %s", cmd.Type, string(cmd.Data))
		resp, _ := json.Marshal(commandResponse{
			Type: cmd.Type + "Response", Success: true, Message: "OK", RequestId: cmd.RequestId,
		})
		enc, _ := box.encrypt(resp)
		wire, _ := json.Marshal(map[string]any{"encrypted": true, "data": enc, "timestamp": time.Now().Unix()})
		if err := conn.WriteMessage(websocket.TextMessage, wire); err != nil {
			log.Println("回执失败:", err)
			return
		}
	}
}
