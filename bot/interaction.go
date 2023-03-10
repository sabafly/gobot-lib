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
	"errors"
	"fmt"

	"github.com/bwmarrin/discordgo"
	"github.com/sabafly/gobot-lib/env"
	"github.com/sabafly/gobot-lib/logging"
)

// アプリケーションコマンドとそのハンダラを備えた構造体
type ApplicationCommand struct {
	*discordgo.ApplicationCommand
	Handler func(*discordgo.Session, *discordgo.InteractionCreate)
}

// アプリケーションコマンドのスライス型
type ApplicationCommands []*ApplicationCommand

// アプリケーションコマンドを解析してハンダラを返す
func (a *ApplicationCommands) Parse() func(*discordgo.Session, *discordgo.InteractionCreate) {
	handler := map[string]func(*discordgo.Session, *discordgo.InteractionCreate){}
	for _, ac := range *a {
		if ac.Handler != nil {
			handler[ac.Name] = ac.Handler
		}
	}
	return func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Type == discordgo.InteractionApplicationCommand {
			if f, ok := handler[i.Interaction.ApplicationCommandData().Name]; ok {
				f(s, i)
			} else {
				logging.Warning("不明なコマンド要求")
			}
		}
	}
}

// ボットにアプリケーションコマンドを登録する
func (b *BotManager) ApplicationCommandCreate(tree ApplicationCommands) (registeredCommands []*discordgo.ApplicationCommand, err error) {
	if len(b.Shards) == 0 {
		return nil, errors.New("error: no session")
	}
	for _, v := range tree {
		cmd, err := b.Shards[0].Session.ApplicationCommandCreate(b.Shards[0].Session.State.User.ID, "", v.ApplicationCommand)
		if err != nil {
			return nil, fmt.Errorf("error: failed to create %s command: %w", v.Name, err)
		}
		if cmd != nil {
			registeredCommands = append(registeredCommands, cmd)
		}
	}
	return registeredCommands, nil
}

// ボットからアプリケーションコマンドを削除する
func (b *BotManager) ApplicationCommandDelete(cmd []*discordgo.ApplicationCommand) (err error) {
	if len(b.Shards) == 0 {
		return errors.New("error: no session")
	}
	for _, ac := range cmd {
		if ac == nil {
			logging.Error("コマンドがnil")
			return errors.New("error: nil command")
		}
		err := b.Shards[0].Session.ApplicationCommandDelete(b.Shards[0].Session.State.User.ID, "", ac.ID)
		if err != nil {
			return err
		}
	}
	return nil
}

// ボットに登録されているコマンドを取得する
func (b *BotManager) ApplicationCommands() ([]*discordgo.ApplicationCommand, error) {
	if len(b.Shards) == 0 {
		return nil, errors.New("error: no session")
	}
	cmd, err := b.Shards[0].Session.ApplicationCommands(b.Shards[0].Session.State.User.ID, "")
	if err != nil {
		return nil, err
	}
	return cmd, nil
}

// サポートサーバーからすべてのコマンドを削除する
//
// TODO: 消すか残りも実装するか
func (b *BotManager) LocalApplicationCommandDelete() error {
	if len(b.Shards) == 0 {
		return errors.New("error: no session")
	}
	cmd, err := b.Shards[0].Session.ApplicationCommands(b.Shards[0].Session.State.User.ID, env.AdminID)
	if err != nil {
		return err
	}
	for _, ac := range cmd {
		if ac == nil {
			logging.Error("コマンドがnil")
			return errors.New("error: nil command")
		}
		err := b.Shards[0].Session.ApplicationCommandDelete(b.Shards[0].Session.State.User.ID, env.AdminID, ac.ID)
		if err != nil {
			return err
		}
	}
	return nil
}
