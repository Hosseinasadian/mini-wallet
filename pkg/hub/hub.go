package hub

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/hosseinasadian/mini-wallet/pkg/redis"
)

const (
	onlinePrefix      = "online:"
	notifyPrefix      = "notify:"
	onlineTTL         = 30 * time.Second
	heartbeatInterval = 10 * time.Second
)

type Hub struct {
	mu     sync.RWMutex
	online map[string]chan string
	rdb    *redis.Redis
}

func NewHub(rdb *redis.Redis) *Hub {
	return &Hub{
		online: make(map[string]chan string),
		rdb:    rdb,
	}
}

func (h *Hub) Connect(userID string) chan string {
	ch := make(chan string, 10)

	h.mu.Lock()
	if oldCh, ok := h.online[userID]; ok {
		close(oldCh)
	}
	h.online[userID] = ch
	h.mu.Unlock()

	h.rdb.Client().Set(context.Background(), onlinePrefix+userID, "1", onlineTTL)

	return ch
}

func (h *Hub) Disconnect(userID string) {
	h.mu.Lock()
	delete(h.online, userID)
	h.mu.Unlock()

	h.rdb.Client().Del(context.Background(), onlinePrefix+userID)
}

func (h *Hub) IsOnline(userID string) bool {
	val, err := h.rdb.Client().Get(context.Background(), onlinePrefix+userID).Result()
	return err == nil && val == "1"
}

func (h *Hub) Publish(userID string, msg string) {
	h.rdb.Client().Publish(context.Background(), notifyPrefix+userID, msg)
}

func (h *Hub) StartListener(ctx context.Context) {
	pubsub := h.rdb.Client().PSubscribe(ctx, notifyPrefix+"*")
	defer pubsub.Close()

	for {
		select {
		case msg := <-pubsub.Channel():
			userID := strings.TrimPrefix(msg.Channel, notifyPrefix)

			h.mu.RLock()
			ch, ok := h.online[userID]
			h.mu.RUnlock()

			if ok {
				select {
				case ch <- msg.Payload:
				default:
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

func (h *Hub) StartHeartbeat(ctx context.Context) {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.mu.RLock()
			userIDs := make([]string, 0, len(h.online))
			for userID := range h.online {
				userIDs = append(userIDs, userID)
			}
			h.mu.RUnlock()

			for _, userID := range userIDs {
				h.rdb.Client().Set(ctx, onlinePrefix+userID, "1", onlineTTL)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for userID := range h.online {
		close(h.online[userID])
		h.rdb.Client().Del(context.Background(), onlinePrefix+userID)
		delete(h.online, userID)
	}
}
