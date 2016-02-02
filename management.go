package main

import (
	"database/sql"
	"net/rpc"
	"github.com/nlopes/slack"
	"fmt"
	_ "github.com/lib/pq"
	"errors"
	"github.com/Sirupsen/logrus"
)

// This file contains the internal management definitions. Everything
// should be called through a method of Manager

type (
	AddTeamReq struct {
		AuthToken string
	}

	AddTeamResp struct {
		Name string
		Url string
		Username string
	}

	Manager interface {
		AddTeam(AddTeamReq, *AddTeamResp) error
	}

	LocalManager struct {
		db *sql.DB
	}

	RemoteManager struct {
		client *rpc.Client
	}
)

// Assert that these two types implement Manager.
var _ Manager = &LocalManager{}
var _ Manager = &RemoteManager{}



func NewLocal(dbpath string) (Manager, error) {
	db, err := sql.Open("postgres", dbpath)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to connect to db")
		return nil, err
	}
	return &LocalManager{db: db}, nil
}

// Makes the calling convention easier.
type ManagementInterface struct{
	Mgr Manager
}

// AddTeam
func (mi ManagementInterface) AddTeam(req AddTeamReq) (*AddTeamResp, error) {
	resp := &AddTeamResp{}
	if err := mi.Mgr.AddTeam(req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (rm *RemoteManager) AddTeam(req AddTeamReq, resp *AddTeamResp) error {
	return rm.client.Call("Manager.AddTeam", req, resp)
}

func (lm *LocalManager) AddTeam(req AddTeamReq, resp *AddTeamResp) error {
	client := slack.New(req.AuthToken)
	authResp, err := client.AuthTest()
	if err != nil {
		return err
	}
	var res sql.Result
	tx, err := lm.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err = tx.Exec(
		"INSERT INTO teams (team_id, team_name, team_authtoken) " +
		"VALUES ($1,$2,$3)" +
		"ON CONFLICT (team_id) DO UPDATE SET (team_name, team_authtoken) = ($2, $3)",
		authResp.TeamID,
		authResp.Team,
		req.AuthToken)
	if err != nil {
		return err
	}

	if nrows, err := res.RowsAffected(); err != nil {
		return err
	} else if nrows != 1 {
		return errors.New("No rows affected")
	}
	fmt.Printf("%#v\n", authResp)
	resp.Name = authResp.Team
	resp.Url = authResp.URL
	resp.Username = authResp.User
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (lm *LocalManager) GetDB() *sql.DB {
	return lm.db
}