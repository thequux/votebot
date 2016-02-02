package main

import (
	"database/sql"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/nlopes/slack"
	"regexp"
)

type Session interface {
	Run()
}
type SlackSession struct {
	Token  string
	TeamID string
	rtm    slack.RTM
	Db     *sql.DB
	log    *log.Entry
}

const rxsProposalName = `[-a-zA-Z0-9_]+`
var (
	rxProposal = regexp.MustCompile(`^propose: (` + rxsProposalName + `)(?: (.*))?`)
	rxVote = regexp.MustCompile(`^ *([-+](?:[1-9][0-9]*(?:\.[0-9]*)?|\.[0-9]+)) on (` + rxsProposalName + `)(?:(.*))?`)
)

func (s *SlackSession) Run() {
	client := slack.New(s.Token)
	rtm := client.NewRTM()
	s.log = log.WithField("token", s.Token)

	log.SetLevel(log.DebugLevel)
	s.log.Info("RTM session ready")
	go rtm.ManageConnection()
Loop:
	for {
		select {
		case msg := <-rtm.IncomingEvents:
			s.log.WithField("type", msg.Type).Debug(
				"Received event: " + fmt.Sprintf("%#v", msg.Data))
			switch ev := msg.Data.(type) {
			case *slack.ConnectedEvent:
				s.log = log.WithFields(log.Fields{
					"team":   rtm.GetInfo().Team.Name,
					"myuser": rtm.GetInfo().User.Name,
				})
				s.TeamID = rtm.GetInfo().Team.ID
				s.updateTeam(rtm.GetInfo().Team)
				for _, user := range rtm.GetInfo().Users {
					s.updateUser(user)
				}
				s.log.Info("Connected")
			case *slack.HelloEvent:
				// Ignore hellos
			case *slack.InvalidAuthEvent:
				s.log.Error("Invalid credentials", ev)
				break Loop
			case *slack.RTMError:
				s.log.WithFields(log.Fields{"code": ev.Code, "msg": ev.Msg}).Warn("RTM error")
			case *slack.UnmarshallingErrorEvent:
				s.log.WithError(ev).Info("Unmarshalling error")
			case *slack.UserChangeEvent:
				s.updateUser(ev.User)
			case *slack.MessageEvent:
				s.handleMessage(ev.User, ev.Channel, &ev.Msg)
			}
		}
	}
}


func (s *SlackSession) handleMessage(User string, Channel string, msg *slack.Msg) {
	if match := rxProposal.FindStringSubmatch(msg.Text); match != nil {
		s.log.WithField("matchgroups", fmt.Sprintf("%#v", match)).Info("Saw proposal")
		topic := match[1]
		comment := match[2]

	} else if match := rxVote.FindStringSubmatch(msg.Text); match != nil {
		s.log.WithField("matchgroups", fmt.Sprintf("%#v", match)).Info("Saw vote")
	}

}
/* Sample message
var foo = &slack.MessageEvent{Msg: slack.Msg{
	Type:      "message",
	Channel:   "C0KGNAP7A",
	User:      "U0E1XSMEG",
	Text:      "More testage",
	Timestamp: "1454444512.000011",
	IsStarred: false, PinnedTo: []string(nil),
	Attachments: []slack.Attachment(nil), Edited: (*slack.Edited)(nil),
	SubType: "", Hidden: false,
	DeletedTimestamp: "",
	EventTimestamp:   "", BotID: "", Username: "", Icons: (*slack.Icon)(nil),
	Inviter: "", Topic: "", Purpose: "", Name: "", OldName: "", Members: []string(nil),
	File: (*slack.File)(nil), Upload: false, Comment: (*slack.Comment)(nil),
	ItemType: "", ReplyTo: 0, Team: "T06RWKEF3"}}
*/

func (s *SlackSession) updateUser(user slack.User) {
	tx, err := s.Db.Begin()
	if err != nil {
		goto fail
	}
	if _, err = tx.Exec(
		"INSERT INTO users (team_id, user_id, user_name) VALUES ($3, $2, $1) "+
			"ON CONFLICT (team_id, user_id) DO UPDATE SET (user_name) = ($1)",
		user.Name, user.ID, s.TeamID); err != nil {

		goto fail
	}
	if err = tx.Commit(); err != nil {
		goto fail
	}
	return

fail:
	tx.Rollback()
	if err != nil {
		s.log.WithError(err).WithField("user", user.ID).Error("Failed to refresh user in db")
	}

}

func (s *SlackSession) updateTeam(team *slack.Team) {
	tx, err := s.Db.Begin()
	if err != nil {
		goto fail
	}
	if _, err = tx.Exec("UPDATE teams SET (team_name) = ($1) WHERE team_id = $2", team.Name, team.ID); err != nil {
		goto fail
	}
	if err = tx.Commit(); err != nil {
		goto fail
	}
	return

fail:
	tx.Rollback()
	if err != nil {
		s.log.WithError(err).Error("Failed to refresh team in db")
	}

}
