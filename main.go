package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/asb1302/innopolis_go_chat/pkg/chatdata"
	"github.com/google/uuid"
)

const (
	pongWait = 30 * time.Second
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGTERM, syscall.SIGINT)
		<-c
		log.Println("Получен syscall.SIGTERM или syscall.SIGINT. Завершаем чат.")

		cancel()
	}()

	run(ctx)
}

func run(ctx context.Context) {
	InitConfig()
	cfg := GetConfig()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			log.Println("Connecting to server:", cfg.ServerHost)

			header := http.Header{}
			header.Add("Authorization", cfg.AuthToken)

			conn, _, err := websocket.DefaultDialer.Dial(cfg.ServerHost, header)
			if err != nil {
				log.Print(err)
				time.Sleep(5 * time.Second)
				continue
			}

			defer conn.Close()

			conn.SetPongHandler(func(appData string) error {
				conn.SetReadDeadline(time.Now().Add(pongWait))
				return nil
			})

			UID := chatdata.ID(uuid.New().String())

			for {
				choice := getUserInput(ctx, "1. Создать новый чат с другим пользователем\n2. Войти в чат с пользователем\n\nВведите ваш выбор (для выхода введите exit или нажмите ctrl+C):\n> ")
				if choice == "" {
					return
				}

				switch choice {
				case "1":
					createNewChat(conn, UID, ctx)
				case "2":
					enterChat(conn, UID, ctx)
				case "exit":
					fmt.Println("Выход из программы")
					return
				default:
					fmt.Println("Неверный выбор. Попробуйте снова.")
				}
			}
		}
	}
}

func getUserInput(ctx context.Context, prompt string) string {
	inputChan := make(chan string)
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		fmt.Print(prompt)
		if scanner.Scan() {
			inputChan <- scanner.Text()
		} else {
			close(inputChan)
		}
	}()

	select {
	case <-ctx.Done():
		return ""
	case input := <-inputChan:
		return input
	}
}

func createNewChat(conn *websocket.Conn, UID chatdata.ID, ctx context.Context) {
	userID := getUserInput(ctx, "Введите ID пользователя, с которым вы бы хотели начать чат\nили введите return для выхода в предыдущее меню.\n> ")

	if userID == "return" || userID == "" {
		return
	}

	chreq := chatdata.NewChatRequest{
		UserIDs: []chatdata.ID{UID, chatdata.ID(userID)},
	}
	req := chatdata.Request{
		Type: chatdata.ReqTypeNewChat,
	}

	data, err := json.Marshal(chreq)
	if err != nil {
		return
	}
	req.Data = data

	if err := conn.WriteJSON(req); err != nil {
		return
	}

	var resp chatdata.Delivery
	if err := conn.ReadJSON(&resp); err != nil {
		return
	}

	chatID, ok := resp.Data.(string)
	if !ok {
		return
	}

	fmt.Printf("Чат создан, ID чата: %s\n", chatID)
}

func enterChat(conn *websocket.Conn, UID chatdata.ID, ctx context.Context) {
	chatID := getUserInput(ctx, "Введите ID чата для начала общения\nили введите return для выхода в предыдущее меню.\n> ")

	if chatID == "return" || chatID == "" {
		return
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				conn.Close()
				return
			default:
				conn.SetReadDeadline(time.Now().Add(pongWait))

				var resp chatdata.Delivery
				if err := conn.ReadJSON(&resp); err != nil {
					conn.Close()
					return
				}
				if msgData, ok := resp.Data.(map[string]interface{}); ok {
					if respChID, ok := msgData["ch_id"].(string); ok && respChID == chatID {
						fmt.Printf("%s %s: %s\n", msgData["from_id"], time.Now().Format("02.01.2024 15:04"), msgData["body"])
					}
				}
			}
		}
	}()

	for {
		message := getUserInput(ctx, "Вводите сообщения для отправки:\n> ")

		if message == "" {
			return
		}

		msg := chatdata.Message{
			MsgID:  chatdata.ID(uuid.New().String()),
			Body:   message,
			TDate:  time.Now(),
			FromID: UID,
		}

		chreq := chatdata.MessageChatRequest{
			Msg:  msg.Body,
			Type: chatdata.MsgTypeAdd,
			ChID: chatdata.ID(chatID),
		}

		data, err := json.Marshal(chreq)
		if err != nil {
			return
		}
		req := chatdata.Request{
			Type: chatdata.ReqTypeNewMsg,
			Data: data,
		}

		if err := conn.WriteJSON(req); err != nil {
			return
		}
	}
}
