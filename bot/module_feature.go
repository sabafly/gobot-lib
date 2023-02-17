package botlib

import (
	"errors"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/google/uuid"
	"github.com/sabafly/gobot-lib/logging"
)

var (
	ErrNoFeatureData = errors.New("no feature data")
)

type FeatureManager struct {
	sync.Mutex
	featureMap                 map[FeatureType][]*Feature
	ApplicationCommandSettings FeatureApplicationCommandSettings
}

type FeatureApplicationCommandSettings struct {
	Name                    string
	Description             string
	NameLocalization        *map[discordgo.Locale]string
	DescriptionLocalization *map[discordgo.Locale]string
	Permission              int64
	DMPermission            bool
	Type                    discordgo.ApplicationCommandType
}

func NewFeatureManager() *FeatureManager {
	return &FeatureManager{
		featureMap: map[FeatureType][]*Feature{},
	}
}

func (fm *FeatureManager) AddFeature(f *Feature) (err error) {
	fm.Lock()
	defer fm.Unlock()
	if f.Data == nil {
		return ErrNoFeatureData
	}
	fm.featureMap[f.Type] = append(fm.featureMap[f.Type], f)
	return nil
}

type FeatureIDType string

const (
	FeatureChannelID FeatureIDType = "CHANNEL_ID"
	FeatureUserID    FeatureIDType = "USER_ID"
	FeatureGuildID   FeatureIDType = "GUILD_ID"
	FeatureRoleID    FeatureIDType = "ROLE_ID"
)

type Feature struct {
	Name         string
	ID           string
	IDType       FeatureIDType
	ChannelTypes []discordgo.ChannelType
	Type         FeatureType
	Data         FeatureData
	Handler      any
}

type FeatureType string

const (
	FeatureMessageCreate FeatureType = "MESSAGE_CREATE"
	FeatureTypingStart               = "TYPING_START"
	FeatureCustom                    = "CUSTOM"
	FeatureUnknown       FeatureType = ""
)

func (f FeatureType) String() string {
	if s, ok := Features[f]; ok {
		return s
	}
	return FeatureUnknown.String()
}

var Features = map[FeatureType]string{
	FeatureMessageCreate: "Message Create",
	FeatureTypingStart:   "Typing Start",
	FeatureCustom:        "Custom",
	FeatureUnknown:       "Unknown",
}

type FeatureData interface {
	Write(string)
	Delete(string)
	IsEnabled(string) bool
}

func (fm *FeatureManager) Handler() func(*discordgo.Session, any) {
	return func(s *discordgo.Session, a any) {
		switch v := a.(type) {
		case *discordgo.MessageCreate:
			for _, f := range fm.featureMap[FeatureMessageCreate] {
				fn, ok := f.Handler.(func(*discordgo.Session, *discordgo.MessageCreate))
				if !ok {
					continue
				}
				var equal bool
				switch f.IDType {
				case FeatureChannelID:
					equal = f.Data.IsEnabled(v.ChannelID)
				case FeatureGuildID:
					equal = f.Data.IsEnabled(v.GuildID)
				case FeatureUserID:
					if v.Member != nil {
						equal = f.Data.IsEnabled(v.Member.User.ID)
					}
				case FeatureRoleID:
					m, err := s.GuildMember(v.GuildID, v.GuildID)
					if err != nil {
						continue
					}
					for _, r := range m.Roles {
						if f.Data.IsEnabled(r) {
							equal = true
							break
						}
					}
				}
				if !equal {
					continue
				}
				fn(s, v)
			}
		case *discordgo.TypingStart:
			for _, f := range fm.featureMap[FeatureTypingStart] {
				fn, ok := f.Handler.(func(*discordgo.Session, *discordgo.TypingStart))
				if !ok {
					continue
				}
				var equal bool
				switch f.IDType {
				case FeatureChannelID:
					equal = f.Data.IsEnabled(v.ChannelID)
				case FeatureGuildID:
					equal = f.Data.IsEnabled(v.GuildID)
				case FeatureUserID:
					equal = f.Data.IsEnabled(v.UserID)
				case FeatureRoleID:
					m, err := s.GuildMember(v.GuildID, v.GuildID)
					if err != nil {
						continue
					}
					for _, r := range m.Roles {
						if f.Data.IsEnabled(r) {
							equal = true
							break
						}
					}
				}
				if !equal {
					continue
				}
				fn(s, v)
			}
		}
	}
}

func (fm *FeatureManager) ApplicationCommand() *discordgo.ApplicationCommand {
	choices := []*discordgo.ApplicationCommandOptionChoice{}
	for _, v := range fm.featureMap {
		for _, f := range v {
			choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
				Name:  f.Name,
				Value: string(f.Type) + f.ID,
			})
		}
	}
	options := []*discordgo.ApplicationCommandOption{
		{
			Name:        "enable",
			Description: "enable feature",
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "feature",
					Description: "the feature to enable",
					Type:        discordgo.ApplicationCommandOptionString,
					Choices:     choices,
				},
			},
		},
	}
	return &discordgo.ApplicationCommand{
		Name:                     fm.ApplicationCommandSettings.Name,
		Description:              fm.ApplicationCommandSettings.Description,
		NameLocalizations:        fm.ApplicationCommandSettings.NameLocalization,
		DescriptionLocalizations: fm.ApplicationCommandSettings.DescriptionLocalization,
		DefaultMemberPermissions: &fm.ApplicationCommandSettings.Permission,
		DMPermission:             &fm.ApplicationCommandSettings.DMPermission,
		Type:                     fm.ApplicationCommandSettings.Type,
		Options:                  options,
	}
}

func (fm *FeatureManager) ApplicationCommandHandler() func(*discordgo.Session, *discordgo.InteractionCreate) {
	return func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Type != discordgo.InteractionApplicationCommand {
			return
		}
		acd := i.ApplicationCommandData()
		if acd.Name != fm.ApplicationCommand().Name {
			return
		}
		option := acd.Options[0]
		switch option.Name {
		case "enable":
			option = option.Options[0]
			var featureID string
			var featureTypeStr string
			var featureType FeatureType
			switch option.Name {
			case "feature":
				featureID = option.StringValue()
				ids := strings.Split(featureID, ":")
				if len(ids) != 0 {
					featureTypeStr = ids[0]
					featureID = ids[len(ids)-1]
				}
				featureType = FeatureType(featureTypeStr)
			}
			if featureID == "" {
				// IDが指定されてないエラー
				embeds := ErrorMessageEmbed(i, "error_invalid_command_argument")
				err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Embeds: embeds,
					},
				})
				if err != nil {
					logging.Error("インタラクションに失敗 %s", err)
				}
				return
			}
			for _, f := range fm.featureMap[featureType] {
				if f.ID != featureID {
					continue
				}
				var values []string
				i, values = RequestFeatureIDRespond(s, i, f)
				for _, v := range values {
					f.Data.Write(v)
				}
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "OK",
					},
				})
			}
		}
	}
}

func RequestFeatureIDRespond(s *discordgo.Session, i *discordgo.InteractionCreate, f *Feature) (ic *discordgo.InteractionCreate, fID []string) {
	var menuType discordgo.SelectMenuType
	channelTypes := []discordgo.ChannelType{}
	switch f.IDType {
	case FeatureChannelID:
		menuType = discordgo.ChannelSelectMenu
		channelTypes = f.ChannelTypes
	case FeatureRoleID:
		menuType = discordgo.RoleSelectMenu
	case FeatureUserID:
		menuType = discordgo.UserSelectMenu
	case FeatureGuildID:
		return i, []string{i.GuildID}
	case FeatureCustom:
		// TODO: 実装する
	}
	// UUID: かぶらないよね
	sessionID := uuid.NewString()
	components := []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					MenuType:     menuType,
					ChannelTypes: channelTypes,
					CustomID:     sessionID,
				},
			},
		},
	}
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags:      discordgo.MessageFlagsEphemeral,
			Components: components,
		},
	})
	if err != nil {
		logging.Error("レスポンスに失敗 %s", err)
		return
	}
	var i1 *discordgo.InteractionCreate
	// TODO: タイムアウトを追加
	var c chan struct{}
	var handler func(*discordgo.Session, *discordgo.InteractionCreate)
	handler = func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Type == discordgo.InteractionMessageComponent {
			s.AddHandlerOnce(handler)
			return
		}
		if i.MessageComponentData().CustomID != sessionID {
			s.AddHandlerOnce(handler)
			return
		}
		i1 = i
		close(c)
	}
	s.AddHandlerOnce(handler)
	<-c
	return i1, i1.MessageComponentData().Values
}
