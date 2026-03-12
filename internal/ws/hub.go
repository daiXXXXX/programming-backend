package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/daiXXXXX/programming-backend/internal/models"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // 开发阶段允许所有来源，生产环境应限制
	},
}

// Client 表示一个 WebSocket 连接的客户端
type Client struct {
	Hub      *Hub
	Conn     *websocket.Conn
	Send     chan []byte
	UserID   int64
	Username string
	Avatar   string
	// 客户端订阅的频道
	Channels map[string]bool
	mu       sync.RWMutex
}

// Hub 管理所有 WebSocket 连接
type Hub struct {
	// 所有已注册的客户端
	clients map[*Client]bool
	// 按用户ID索引的客户端
	userClients map[int64][]*Client
	// 按频道索引的客户端
	channelClients map[string]map[*Client]bool
	// 注册请求
	register chan *Client
	// 注销请求
	unregister chan *Client
	// 广播消息到所有客户端
	broadcast chan []byte
	// 频道消息
	channelMsg chan *ChannelMessage
	mu         sync.RWMutex
}

// ChannelMessage 频道消息
type ChannelMessage struct {
	Channel string
	Data    []byte
}

// NewHub 创建新的 Hub
func NewHub() *Hub {
	return &Hub{
		clients:        make(map[*Client]bool),
		userClients:    make(map[int64][]*Client),
		channelClients: make(map[string]map[*Client]bool),
		register:       make(chan *Client),
		unregister:     make(chan *Client),
		broadcast:      make(chan []byte),
		channelMsg:     make(chan *ChannelMessage, 256),
	}
}

// Run 启动 Hub 事件循环
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.userClients[client.UserID] = append(h.userClients[client.UserID], client)
			h.mu.Unlock()
			log.Printf("[WS] Client connected: user=%s(id=%d), total=%d", client.Username, client.UserID, len(h.clients))
			// 广播在线人数
			h.BroadcastOnlineCount()

		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				h.mu.Lock()
				delete(h.clients, client)
				// 从 userClients 中移除
				clients := h.userClients[client.UserID]
				for i, c := range clients {
					if c == client {
						h.userClients[client.UserID] = append(clients[:i], clients[i+1:]...)
						break
					}
				}
				if len(h.userClients[client.UserID]) == 0 {
					delete(h.userClients, client.UserID)
				}
				// 从所有频道中移除
				client.mu.RLock()
				for ch := range client.Channels {
					if clients, ok := h.channelClients[ch]; ok {
						delete(clients, client)
						if len(clients) == 0 {
							delete(h.channelClients, ch)
						}
					}
				}
				client.mu.RUnlock()
				h.mu.Unlock()
				close(client.Send)
				log.Printf("[WS] Client disconnected: user=%s(id=%d), total=%d", client.Username, client.UserID, len(h.clients))
				h.BroadcastOnlineCount()
			}

		case message := <-h.broadcast:
			h.mu.RLock()
			var deadClients []*Client
			for client := range h.clients {
				select {
				case client.Send <- message:
				default:
					deadClients = append(deadClients, client)
				}
			}
			h.mu.RUnlock()
			// 清理断开的客户端（在锁外操作）
			for _, client := range deadClients {
				h.unregister <- client
			}

		case msg := <-h.channelMsg:
			h.mu.RLock()
			var deadChannelClients []*Client
			if clients, ok := h.channelClients[msg.Channel]; ok {
				for client := range clients {
					select {
					case client.Send <- msg.Data:
					default:
						deadChannelClients = append(deadChannelClients, client)
					}
				}
			}
			h.mu.RUnlock()
			for _, client := range deadChannelClients {
				h.unregister <- client
			}
		}
	}
}

// Subscribe 让客户端订阅某个频道
func (h *Hub) Subscribe(client *Client, channel string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	client.mu.Lock()
	client.Channels[channel] = true
	client.mu.Unlock()

	if _, ok := h.channelClients[channel]; !ok {
		h.channelClients[channel] = make(map[*Client]bool)
	}
	h.channelClients[channel][client] = true
	log.Printf("[WS] Client %s subscribed to channel: %s", client.Username, channel)
}

// Unsubscribe 让客户端取消订阅某个频道
func (h *Hub) Unsubscribe(client *Client, channel string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	client.mu.Lock()
	delete(client.Channels, channel)
	client.mu.Unlock()

	if clients, ok := h.channelClients[channel]; ok {
		delete(clients, client)
		if len(clients) == 0 {
			delete(h.channelClients, channel)
		}
	}
}

// BroadcastToChannel 向指定频道广播消息
func (h *Hub) BroadcastToChannel(channel string, msg *models.WSMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[WS] Failed to marshal message: %v", err)
		return
	}
	h.channelMsg <- &ChannelMessage{Channel: channel, Data: data}
}

// BroadcastToAll 向所有客户端广播消息
func (h *Hub) BroadcastToAll(msg *models.WSMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[WS] Failed to marshal message: %v", err)
		return
	}
	h.broadcast <- data
}

// SendToUser 向指定用户发送消息
func (h *Hub) SendToUser(userID int64, msg *models.WSMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[WS] Failed to marshal message: %v", err)
		return
	}

	h.mu.RLock()
	clients := h.userClients[userID]
	h.mu.RUnlock()

	for _, client := range clients {
		select {
		case client.Send <- data:
		default:
		}
	}
}

// BroadcastOnlineCount 广播当前在线人数
func (h *Hub) BroadcastOnlineCount() {
	h.mu.RLock()
	count := len(h.userClients) // 按用户去重
	h.mu.RUnlock()

	msg := &models.WSMessage{
		Type:      models.WSTypeOnlineCount,
		Content:   map[string]int{"count": count},
		Timestamp: time.Now(),
	}
	data, _ := json.Marshal(msg)
	// 直接发送不经过 broadcast channel，避免死锁
	h.mu.RLock()
	for client := range h.clients {
		select {
		case client.Send <- data:
		default:
		}
	}
	h.mu.RUnlock()
}

// GetOnlineCount 获取在线用户数
func (h *Hub) GetOnlineCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.userClients)
}

// wsClientAction 客户端发送的动作消息
type wsClientAction struct {
	Action  string `json:"action"`  // subscribe, unsubscribe, chat
	Channel string `json:"channel"` // 频道
	Content string `json:"content"` // 消息内容（chat时用）
}

// ReadPump 从 WebSocket 连接读取消息
func (c *Client) ReadPump() {
	defer func() {
		c.Hub.unregister <- c
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(4096)
	c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("[WS] Read error: %v", err)
			}
			break
		}

		// 解析客户端消息
		var action wsClientAction
		if err := json.Unmarshal(message, &action); err != nil {
			log.Printf("[WS] Invalid message format: %v", err)
			continue
		}

		switch action.Action {
		case "subscribe":
			if action.Channel != "" {
				c.Hub.Subscribe(c, action.Channel)
			}
		case "unsubscribe":
			if action.Channel != "" {
				c.Hub.Unsubscribe(c, action.Channel)
			}
		case "chat":
			if action.Channel != "" && action.Content != "" {
				msg := &models.WSMessage{
					Type:    models.WSTypeChat,
					Channel: action.Channel,
					From: &models.SolutionAuthor{
						ID:       c.UserID,
						Username: c.Username,
						Avatar:   c.Avatar,
					},
					Content:   action.Content,
					Timestamp: time.Now(),
				}
				c.Hub.BroadcastToChannel(action.Channel, msg)
			}
		}
	}
}

// WritePump 向 WebSocket 连接写入消息
func (c *Client) WritePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// 将队列中的消息也一并发送
			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.Send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// ServeWS 处理 WebSocket 升级请求
func ServeWS(hub *Hub, w http.ResponseWriter, r *http.Request, userID int64, username, avatar string) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WS] Upgrade error: %v", err)
		return
	}

	client := &Client{
		Hub:      hub,
		Conn:     conn,
		Send:     make(chan []byte, 256),
		UserID:   userID,
		Username: username,
		Avatar:   avatar,
		Channels: make(map[string]bool),
	}

	hub.register <- client

	go client.WritePump()
	go client.ReadPump()
}
