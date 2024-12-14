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

var state ClientState

type ClientState struct {
	currentChatID chatdata.ID
}

func (cs *ClientState) SetChatID(id chatdata.ID) {
	cs.currentChatID = id
}

func (cs *ClientState) GetChatID() chatdata.ID {
	return cs.currentChatID
}

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

	log.Println("Connecting to server:", cfg.ServerHost)

	header := http.Header{}
	header.Add("Authorization", cfg.AuthToken)

	conn, _, err := websocket.DefaultDialer.Dial(cfg.ServerHost, header)
	if err != nil {
		log.Printf("Ошибка подключения к серверу: %v", err)
		return
	}
	defer conn.Close()

	UID := chatdata.ID(uuid.New().String())

	for {
		select {
		case <-ctx.Done():
			log.Println("Завершаем чат, т.к. получен сигнал завершения")
			return
		default:
			choice := getUserInput(ctx, "1. Создать новый чат с другим пользователем\n2. Войти в чат с пользователем\n\nВведите ваш выбор (для выхода введите exit или нажмите ctrl+C):\n> ")
			if choice == "" {
				fmt.Println("Сделайте выбор.")
				continue
			}

			switch choice {
			case "1":
				createNewChat(conn, UID, ctx)
			case "2":
				// Запускаем горутину чтения только после того, как пользователь вошел в чат. Упрощённый пример.
				go readMessages(ctx, conn)
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

// читает все сообщения и выводит только те, что относятся к текущему чату
func readMessages(ctx context.Context, conn *websocket.Conn) {
	for {
		select {
		case <-ctx.Done():
			conn.Close()
			return
		default:
			// чтобы соединение не повисло при отсутствии данных
			conn.SetReadDeadline(time.Now().Add(pongWait))

			var resp chatdata.Delivery
			if err := conn.ReadJSON(&resp); err != nil {
				log.Printf("Ошибка чтения ws: %v\n", err)
				conn.Close()
				return
			}

			msgData, ok := resp.Data.(map[string]interface{})
			if !ok {
				continue
			}

			respChID, ok := msgData["ch_id"].(string)
			if !ok {
				continue
			}

			if chatdata.ID(respChID) == state.GetChatID() {
				fmt.Printf("%s %s: %s\n",
					msgData["from_id"],
					time.Now().Format("02.01.2006 15:04"),
					msgData["body"])
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

	// Устанавливаем текущий чат для филтрации в readMessages
	state.SetChatID(chatdata.ID(chatID))

	for {
		message := getUserInput(ctx, "Вводите сообщения для отправки:\n> ")
		if message == "" {
			log.Println("Пустое сообщение. Возвращаемся в меню.")
			state.SetChatID("") // сбрасываем текущий чат
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
			log.Printf("Ошибка сериализации сообщения: %v\n", err)
			continue
		}

		req := chatdata.Request{
			Type: chatdata.ReqTypeNewMsg,
			Data: data,
		}

		if err := conn.WriteJSON(req); err != nil {
			log.Printf("Ошибка отправки сообщения: %v\n", err)
			state.SetChatID("") // и снова сбрасываем при ошибке
			return
		}
	}
}
