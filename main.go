package main

import (
	"github.com/codegangsta/cli"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	"flag"
	"fmt"
	"os"
	"github.com/Sirupsen/logrus"
)

var (
	dbPath = flag.String("db", "votebot.sqlite3", "Database file")
)


func main() {

	var manager ManagementInterface

	app := cli.NewApp()
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name: "db",
			Value: "dbname=votebot",
			Usage: "Postgresql connection string",
		},
	}
	app.Before = func(ctx *cli.Context) error {
		mgr, err := NewLocal(ctx.String("db"))
		if err != nil {
			fmt.Printf("Failed to connect to database")
			return err
		}
		manager = ManagementInterface{mgr}
		return nil
	}
	app.Commands = []cli.Command{
		{
			Name: "connect",
			Usage: "Add a team",
			ArgsUsage: "[authtoken]",
			Action: func(ctx *cli.Context) {
				if len(ctx.Args()) != 1 {
					fmt.Printf("Usage: connect [authid]")
					return
				}
				res, err := manager.AddTeam(AddTeamReq{
					AuthToken: ctx.Args()[0],
				})
				if err != nil {
					fmt.Printf("Error: %s", err)
				} else {
					fmt.Printf("Connected to team %s as %s\n", res.Name, res.Username)
				}
			},
		},
		{
			Name: "daemon",
			Usage: "Perform the botly duties",
			Action: func(ctx *cli.Context) {
				if len(ctx.Args()) != 0 {
					fmt.Printf("Usage: daemon")
					return
				}
				RunSessions(manager)
			},
		},
	}

	app.Run(os.Args)
}

func RunSessions(mgmt ManagementInterface) {
	db := mgmt.Mgr.(*LocalManager).GetDB()
	sessions := []*SlackSession{}
	tx, err := db.Begin()
	if err != nil {
		logrus.WithError(err).Fatal("Failed to begin query txn")
	}
	defer tx.Rollback()
	rows, err := tx.Query("SELECT team_id, team_authtoken FROM teams")
	if err != nil {
		fmt.Printf("Failed to list teams: %s", err)
		return
	}
	for rows.Next() {
		var (
			id string
			slackToken string
		)
		if err := rows.Scan(&id, &slackToken); err != nil {
			fmt.Printf("Failed to read info for row: %s", err)
		}
		session := &SlackSession{Db: db, Token: slackToken, TeamID: id}
		sessions = append(sessions, session)
		go session.Run()
	}

	<- make(chan bool, 1)
}