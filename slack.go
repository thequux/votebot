package main

import (
	"database/sql"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/nlopes/slack"
	"regexp"
	"github.com/shopspring/decimal"
	"github.com/anmitsu/go-shlex"
	"strings"
	"text/template"
	"bytes"
)

const voteDecimalPlaces = 2
type Session interface {
	Run()
}
type SlackSession struct {
	Token  string
	TeamID string
	rtm    *slack.RTM
	Db     *sql.DB
	log    *log.Entry
	rxAtMessage *regexp.Regexp
}

const rxsProposalName = `[-a-zA-Z0-9_]+`
var (
	rxProposal = regexp.MustCompile(`^propose (` + rxsProposalName + `)(?:[:;,] *(.*))?`)
	rxVote = regexp.MustCompile(`^ *([-+](?:[1-9][0-9]*(?:\.[0-9]*)?|\.[0-9]+)) on (` + rxsProposalName + `)(?: *[;:,] *(.*))?`)

)

var topicSummary = template.Must(template.New("root").Parse(`{{if .}} Vote summary
{{end -}}
{{- range . -}}
* {{.name}} -- {{.total}} ({{.nvotes}} votes) | {{.comment}}
{{ else -}}
No open topics
{{- end -}}
`))

func (s *SlackSession) Run() {
	client := slack.New(s.Token)
	//client.SetDebug(true)
	rtm := client.NewRTM()
	s.rtm = rtm
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
				s.rxAtMessage = regexp.MustCompile(fmt.Sprintf(`^(?:<@%s>: |%s |%s: )(.*)`,
					regexp.QuoteMeta(rtm.GetInfo().User.ID),
					regexp.QuoteMeta(rtm.GetInfo().User.Name),
					regexp.QuoteMeta(rtm.GetInfo().User.Name)))
				for _, user := range rtm.GetInfo().Users {
					s.updateUser(user)
				}
				/*
				pmp := slack.NewPostMessageParameters()
				if _,_,err := rtm.PostMessage("#botdev", "votebot here", pmp); err != nil {
					log.WithError(err).Warn("Failed to post intro message")
				}
				*/
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
	tx, err := s.Db.Begin()
	if err != nil {
		goto fail
	}
	defer tx.Rollback()

	// Check user type; ignore bots
	{
		if msg.SubType == "bot_message" {
			return
		}
		var is_bot bool
		if err = tx.QueryRow("select user_is_bot from users where user_id = $1 and team_id = $2",
			User, s.TeamID).Scan(&is_bot); err != nil {
			s.log.WithError(err).Warn("Failed to read row")
			goto fail
		}
		if is_bot {
			s.log.Debug("Ignoring message; from bot")
			return
		}
	}

	if match := rxProposal.FindStringSubmatch(msg.Text); match != nil {
		s.log.WithField("matchgroups", fmt.Sprintf("%#v", match)).Info("Saw proposal")
		topic := match[1]
		comment := match[2]

		_, err = tx.Exec(
			"INSERT INTO topics (team_id, topic_channel, topic_name, topic_comment) "+
			"VALUES ($1, $2, $3, nullif($4,''))",
			s.TeamID, Channel, topic, comment)
		if err != nil {
			// TODO: report error in channel
			s.log.WithError(err).Info("Failed to insert proposal")
			goto fail
		}
		pmp := slack.NewPostMessageParameters()
		pmp.AsUser = true
		//pmp.Username = s.rtm.GetInfo().User.Name
		_,_,err = s.rtm.PostMessage(Channel, fmt.Sprintf("Voting is now open on %s", topic), pmp)
		if err != nil {
			log.WithError(err).Info("Failed to post voting open message")
		}

	} else if match := rxVote.FindStringSubmatch(msg.Text); match != nil {
		var value decimal.Decimal
		topic := match[2]
		log := s.log.WithFields(log.Fields{
			"topic": topic,
			"user": User,
			"channel": msg.Channel,
			"user_msg": msg.Text,
		})
		if value, err = decimal.NewFromString(match[1]); err != nil {
			log.WithError(err).Info("Failed to parse vote value")
			return
		}
		comment := match[3]

		_, err = tx.Exec(
			"INSERT INTO votes (team_id, topic_name, user_id, vote_value, vote_comment) " +
			"VALUES ($1, $2, $3, $4, nullif($5, '')) " +
			"ON CONFLICT (topic_name, team_id, user_id) " +
			"DO UPDATE SET (vote_value, vote_comment) = ($4, nullif($5, ''))",
			s.TeamID, topic, User, value, comment)
		if err != nil {
			log.WithError(err).Info("Failed to insert vote")
		}
		log.WithField("matchgroups", fmt.Sprintf("%#v", match)).Info("Saw vote")
	} else if match := s.rxAtMessage.FindStringSubmatch(msg.Text); match != nil {
		args, err := shlex.Split(match[1], true)
		pmp := slack.NewPostMessageParameters()
		pmp.AsUser = true
		pmp.EscapeText = false
		user_pfx := fmt.Sprintf("<@%s>: ", msg.User)
		if err != nil || len(args) < 1 {
			s.rtm.PostMessage(Channel, user_pfx + " Syntax error: %s" + err.Error(), pmp)
			return
		}
		switch strings.ToLower(args[0]) {
		case "howdy":
			s.rtm.PostMessage(Channel, user_pfx + " Howdy neighbor!", pmp)
		case "status":
			if len(args) < 2 {
				topics := []map[string]interface{}{}
				rows, err := tx.Query(
					"SELECT topic_name, COALESCE(topic_comment, ''), COUNT(user_id), SUM(vote_value) " +
					"FROM topics NATURAL LEFT JOIN (votes NATURAL JOIN users) " +
					"WHERE topic_open AND team_id = $1 AND topic_channel = $2 " +
					"GROUP BY topic_name, topic_channel, team_id, topic_comment",
					s.TeamID, Channel)
				if err != nil {
					log.WithError(err).Error("Failed to query topics")
					return
				}
				for rows.Next() {
					var (
						name, comment string
						count int
						sum decimal.Decimal
					)
					rows.Scan(&name, &comment, &count, &sum)
					topics = append(topics, map[string]interface{}{
						"name": name,
						"comment": comment,
						"nvotes": count,
						"total": sum,
					})
				}
				buffer := &bytes.Buffer{}
				topicSummary.Execute(buffer, topics)
				s.rtm.PostMessage(Channel, buffer.String(), pmp)
			}

		default:
			s.rtm.PostMessage(Channel, fmt.Sprintf("%s You seem confused; you said `%#v`", user_pfx, args), pmp)
		}
	}
	tx.Commit()
	return
	fail:
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
	defer tx.Rollback()
	if _, err = tx.Exec(
		"INSERT INTO users (team_id, user_id, user_name, user_is_bot) VALUES ($3, $2, $1, $4) "+
			"ON CONFLICT (team_id, user_id) DO UPDATE SET (user_name, user_is_bot) = ($1, $4)",
		user.Name, user.ID, s.TeamID, user.IsBot); err != nil {

		goto fail
	}
	if err = tx.Commit(); err != nil {
		goto fail
	}
	return

fail:
	if err != nil {
		s.log.WithError(err).WithField("user", user.ID).Error("Failed to refresh user in db")
	}

}

func (s *SlackSession) updateTeam(team *slack.Team) {
	tx, err := s.Db.Begin()
	if err != nil {
		goto fail
	}
	defer tx.Rollback()
	if _, err = tx.Exec("UPDATE teams SET (team_name) = ($1) WHERE team_id = $2", team.Name, team.ID); err != nil {
		goto fail
	}
	if err = tx.Commit(); err != nil {
		goto fail
	}
	return

fail:
	if err != nil {
		s.log.WithError(err).Error("Failed to refresh team in db")
	}

}
