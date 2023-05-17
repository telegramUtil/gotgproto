package dispatcher

import (
	"context"
	"errors"
	"fmt"
	"log"
	"runtime/debug"

	"github.com/anonyindian/gotgproto/ext"
	"github.com/anonyindian/gotgproto/storage"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"
	"go.uber.org/multierr"
)

var (
	// EndGroups stops iterating over handlers groups if return through handler callback function.
	EndGroups = errors.New("stopped")
	// ContinueGroups continues iterating over handlers groups if return through handler callback function.
	ContinueGroups = errors.New("continued")
	// SkipCurrentGroup skips current group and continues iterating over handlers groups if return through handler callback function.
	SkipCurrentGroup = errors.New("skipped")
)

type Dispatcher interface {
	Initialize(context.Context, *telegram.Client, *tg.User)
	Handle(context.Context, tg.UpdatesClass) error
	AddHandler(Handler)
	AddHandlerToGroup(Handler, int)
}

type NativeDispatcher struct {
	client *tg.Client
	self   *tg.User
	sender *message.Sender
	// Panic handles all the panics that occur during handler execution.
	Panic PanicHandler
	// Error handles all the unknown errors which are returned by the handler callback functions.
	Error ErrorHandler
	// handlerMap is used for internal functionality of NativeDispatcher.
	handlerMap map[int][]Handler
	// handlerGroups is used for internal functionality of NativeDispatcher.
	handlerGroups []int
}

type PanicHandler func(*ext.Context, *ext.Update, string)
type ErrorHandler func(*ext.Context, *ext.Update, string) error

// MakeDispatcher creates new custom dispatcher which process and handles incoming updates.
func NewNativeDispatcher() *NativeDispatcher {
	return &NativeDispatcher{
		handlerMap: make(map[int][]Handler),
	}
}

type entities tg.Entities

func (u *entities) short() {
	u.Short = true
	u.Users = make(map[int64]*tg.User, 0)
	u.Chats = make(map[int64]*tg.Chat, 0)
	u.Channels = make(map[int64]*tg.Channel, 0)
}

func (dp *NativeDispatcher) Initialize(ctx context.Context, client *telegram.Client, self *tg.User) {
	dp.client = client.API()
	dp.sender = message.NewSender(dp.client)
	dp.self = self
}

// Handle function handles all the incoming updates, map entities and dispatches updates for further handling.
func (dp *NativeDispatcher) Handle(ctx context.Context, updates tg.UpdatesClass) error {
	var (
		e    entities
		upds []tg.UpdateClass
	)
	switch u := updates.(type) {
	case *tg.Updates:
		upds = u.Updates
		e.Users = u.MapUsers().NotEmptyToMap()
		chats := u.MapChats()
		e.Chats = chats.ChatToMap()
		e.Channels = chats.ChannelToMap()
		go func() {
			saveUsersPeers(u.Users)
			saveChatsPeers(u.Chats)
		}()
	case *tg.UpdatesCombined:
		upds = u.Updates
		e.Users = u.MapUsers().NotEmptyToMap()
		chats := u.MapChats()
		e.Chats = chats.ChatToMap()
		e.Channels = chats.ChannelToMap()
		go func() {
			saveUsersPeers(u.Users)
			saveChatsPeers(u.Chats)
		}()
	case *tg.UpdateShort:
		upds = []tg.UpdateClass{u.Update}
		e.short()
	default:
		return nil
	}

	var err error
	for _, update := range upds {
		multierr.AppendInto(&err, dp.dispatch(ctx, tg.Entities(e), update))
	}
	return err
}

func (dp *NativeDispatcher) dispatch(ctx context.Context, e tg.Entities, update tg.UpdateClass) error {
	if update == nil {
		return nil
	}
	return dp.handleUpdate(ctx, e, update)
}

func (dp *NativeDispatcher) handleUpdate(ctx context.Context, e tg.Entities, update tg.UpdateClass) error {
	u := ext.GetNewUpdate(ctx, dp.client, &e, update)
	c := ext.NewContext(ctx, dp.client, dp.self, dp.sender, &e)
	var err error
	defer func() {
		if r := recover(); r != nil {
			errorStack := fmt.Sprintf("%s\n", r) + string(debug.Stack())
			if dp.Panic != nil {
				dp.Panic(c, u, errorStack)
				return
			}
			log.Println(errorStack)
		}
	}()
	for _, group := range dp.handlerGroups {
		for _, handler := range dp.handlerMap[group] {
			err = handler.CheckUpdate(c, u)
			if err == nil || errors.Is(err, ContinueGroups) {
				continue
			} else if errors.Is(err, EndGroups) {
				return err
			} else if errors.Is(err, SkipCurrentGroup) {
				break
			} else {
				err = dp.Error(c, u, err.Error())
				switch err {
				case ContinueGroups:
					continue
				case EndGroups:
					return err
				case SkipCurrentGroup:
					break
				}
			}
		}
	}
	return err
}

func saveUsersPeers(u tg.UserClassArray) {
	for _, user := range u {
		c, ok := user.AsNotEmpty()
		if !ok {
			continue
		}
		storage.AddPeer(c.ID, c.AccessHash, storage.TypeUser, c.Username)
	}
}

func saveChatsPeers(u tg.ChatClassArray) {
	for _, chat := range u {
		channel, ok := chat.(*tg.Channel)
		if ok {
			storage.AddPeer(channel.ID, channel.AccessHash, storage.TypeChannel, channel.Username)
			continue
		}
		chat, ok := chat.(*tg.Chat)
		if !ok {
			continue
		}
		storage.AddPeer(chat.ID, storage.DefaultAccessHash, storage.TypeChat, storage.DefaultUsername)
	}
}
