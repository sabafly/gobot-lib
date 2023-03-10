/*
	Copyright (C) 2022-2023  ikafly144

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package botlib

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/gorilla/websocket"
	"github.com/sabafly/gobot-lib/constants"
	"github.com/sabafly/gobot-lib/env"
	"github.com/sabafly/gobot-lib/logging"
)

func init() {
	discordgo.Logger = logging.Logger()
}

// シャードとセッションをまとめる
type Shard struct {
	ShardID int
	*Api
	Session *discordgo.Session
}

type Api struct {
	sync.RWMutex
	Client         http.Client
	MaxRestRetries int
	UserAgent      string

	listening chan any

	handlersMu sync.RWMutex
	handlers   map[string][]*eventHandlerInstance

	Dialer   *websocket.Dialer
	wsConn   *websocket.Conn
	wsMutex  sync.Mutex
	sequence *int64
	gateway  string
}

// ボット接続を管理する
type BotManager struct {
	*Api
	ShardCount int
	Shards     []*Shard
	features   *FeatureManager
}

// ボットセッションを開始する
func (b *BotManager) Open() (err error) {
	shards := b.Shards

	for i := range shards {
		// 内部APIと接続
		if err := shards[i].ApiOpen(); err != nil {
			return fmt.Errorf("failed open api connection: %w", err)
		}

		s := shards[i].Session

		// セッションを初期化
		s.Identify.Intents = discordgo.IntentsAll
		s.ShardCount = b.ShardCount
		s.ShardID = shards[i].ShardID
		s.UserAgent = constants.UserAgent
		s.StateEnabled = true

		s.LogLevel = env.DLogLevel

		// セッションを開始
		if err := s.Open(); err != nil {
			return fmt.Errorf("failed open session: %w", err)
		}
	}
	b.Shards = shards
	return nil
}

// ボットセッションを終了する
func (b *BotManager) Close() (err error) {
	shards := b.Shards
	for i := range shards {
		s := shards[i].Session

		if err := s.Close(); err != nil {
			return fmt.Errorf("failed close session: %w", err)
		}

		if err := shards[i].ApiClose(); err != nil {
			return fmt.Errorf("failed close api connection: %w", err)
		}
	}
	b.Shards = shards
	return nil
}

// 新規のボット接続を作成する
func New(token string) (bot *BotManager, err error) {
	// セッションを作成
	s, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("failed create bot: %w", err)
	}

	// シャードの個数を取得
	count, err := shardCount(s)
	if err != nil {
		return nil, fmt.Errorf("failed get shard count: %w", err)
	}

	// シャードを設定
	return validateShards(token, count)
}

func NewApi() *Api {
	var zero int64 = 0
	return &Api{
		Dialer:         websocket.DefaultDialer,
		MaxRestRetries: 5,
		Client:         http.Client{},
		sequence:       &zero,
	}
}

// シャード数を取得する
func shardCount(s *discordgo.Session) (count int, err error) {
	gateway, err := s.GatewayBot()
	if err != nil {
		return 0, fmt.Errorf("failed request gateway bot: %w", err)
	}
	count = gateway.Shards
	return count, nil
}

// 指定した数のシャードを用意する
func validateShards(token string, count int) (bot *BotManager, err error) {
	bot = &BotManager{
		// API接続関連
		Api:      NewApi(),
		features: NewFeatureManager(),
	}

	for i := 0; i < count; i++ {
		s, err := discordgo.New("Bot " + token)
		if err != nil {
			return nil, fmt.Errorf("failed validate shard %v: %w", i, err)
		}
		s.AddHandler(bot.interfaceHandler)
		bot.Shards = append(bot.Shards, &Shard{
			ShardID: i,
			Session: s,
			// API接続関連
			Api: NewApi(),
		})
	}

	return bot, nil
}

// セッションにDiscordAPIイベントハンダラを登録する
func (b *BotManager) AddHandler(handler any) {
	for _, s := range b.Shards {
		s.Session.AddHandler(handler)
	}
}

func (b *BotManager) interfaceHandler(s *discordgo.Session, i any) {
	switch t := i.(type) {
	case *discordgo.GuildCreate:
		b.guildCreateCall(t.ID)
	case *discordgo.GuildDelete:
		b.guildDeleteCall(t)
	case *discordgo.MessageCreate:
		b.messageCreateCall(t)
	}
}

// 内部APIのイベントハンダラを登録する
func (b *BotManager) AddApiHandler(handler any) {
	for _, s := range b.Shards {
		s.AddHandler(handler)
	}
}

// DiscordAPIイベントから内部APIを呼び出すときに使う
//
// XXX: メソッドで実装したい
func AddIntegrationHandler[T any](b *BotManager, handler func(*Api, *discordgo.Session, T)) {
	b.AddHandler(func(s *discordgo.Session, d T) {
		handler(b.Api, s, d)
	})
}
