package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

var webConn sync.Map

type userSession struct {
	conn     *websocket.Conn
	messages []openai.ChatCompletionMessageParamUnion
	mutex    sync.Mutex
}

func chatUser(session *userSession, prompt string) string {
	client := openai.NewClient(
		option.WithAPIKey(os.Getenv("OPENAI_APIKEY")),
	)

	session.mutex.Lock()
	defer session.mutex.Unlock()

	session.messages = append(session.messages, openai.UserMessage(prompt))
	resp, err := client.Chat.Completions.New(context.TODO(), openai.ChatCompletionNewParams{
		Model:    openai.ChatModelChatgpt4oLatest,
		Messages: session.messages,
	})
	if err != nil {
		log.Println("OpenAI Chat Error:", err)
		return "Sorry, I had an issue processing that."
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	session.messages = append(session.messages, openai.AssistantMessage(content))
	return content
}

func handleClient(userID string, conn *websocket.Conn) {
	defer conn.Close()
	defer webConn.Delete(userID)

	log.Printf("User %s connected", userID)

	session := &userSession{
		conn:     conn,
		messages: []openai.ChatCompletionMessageParamUnion{openai.SystemMessage("You are a sweet AI Girlfriend.")},
	}

	webConn.Store(userID, session)

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Printf("User %s disconnected: %v", userID, err)
			return
		}

		go func(message string) {
			response := chatUser(session, message)
			err := conn.WriteMessage(websocket.TextMessage, []byte(response))
			if err != nil {
				log.Printf("Failed to send to %s: %v", userID, err)
			}
		}(string(msg))
	}
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("uid")
	if userID == "" {
		http.Error(w, "Missing USER ID", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	go handleClient(userID, conn)
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	http.HandleFunc("/ws", wsHandler)
	log.Println("Server started on :8001")
	err = http.ListenAndServe(":8001", nil)
	if err != nil {
		log.Fatalf("ListenAndServe error: %v", err)
	}
}
