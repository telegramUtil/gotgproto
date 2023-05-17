package gotgproto

//go:generate go run ./generator

import (
	"context"
	"fmt"

	"github.com/anonyindian/gotgproto/dispatcher"
	"github.com/anonyindian/gotgproto/sessionMaker"
	"github.com/anonyindian/gotgproto/storage"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/tg"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const VERSION = "v1.0.0-beta10"

type Client struct {
	// Dispatcher handlers the incoming updates and execute mapped handlers. It is recommended to use dispatcher.MakeDispatcher function for this field.
	Dispatcher dispatcher.Dispatcher
	// PublicKeys of telegram.
	//
	// If not provided, embedded public keys will be used.
	PublicKeys []telegram.PublicKey
	// DC ID to connect.
	//
	// If not provided, 2 will be used by default.
	DC int
	// DCList is initial list of addresses to connect.
	DCList dcs.List
	// Resolver to use.
	Resolver dcs.Resolver
	// Whether to show the copyright line in console or no.
	DisableCopyright bool
	// Session info of the authenticated user, use sessionMaker.NewSession function to fill this field.
	Session *sessionMaker.SessionName

	// Self contains details of logged in user in the form of *tg.User.
	Self *tg.User

	clientType ClientType
	ctx        context.Context
	err        error
	started    chan int

	*telegram.Client
}

// Type of client to login to, can be of 2 types:
// 1.) Bot  (Fill BotToken in this case)
// 2.) User (Fill Phone in this case)
type ClientType struct {
	// BotToken is the unique API Token for the bot you're trying to authorize, get it from @BotFather.
	BotToken string
	// Mobile number of the authenticating user.
	Phone string
}

type ClientOpts struct {
	// Logger is instance of zap.Logger. No logs by default.
	Logger *zap.Logger
	// PublicKeys of telegram.
	//
	// If not provided, embedded public keys will be used.
	PublicKeys []telegram.PublicKey
	// DC ID to connect.
	//
	// If not provided, 2 will be used by default.
	DC int
	// DCList is initial list of addresses to connect.
	DCList dcs.List
	// Resolver to use.
	Resolver dcs.Resolver
	// Whether to show the copyright line in console or no.
	DisableCopyright bool
	// Session info of the authenticated user, use sessionMaker.NewSession function to fill this field.
	Session *sessionMaker.SessionName
}

func NewClient(appId int, apiHash string, cType ClientType, opts *ClientOpts) (*Client, error) {
	if opts == nil {
		opts = &ClientOpts{}
	}

	var sessionStorage telegram.SessionStorage
	if opts.Session == nil || opts.Session.GetName() == ":memory:" {
		sessionStorage = &session.StorageMemory{}
		storage.StoreInMemory = true
	} else {
		sessionStorage = &sessionMaker.SessionStorage{
			Session: opts.Session,
		}
	}

	d := dispatcher.NewNativeDispatcher()

	client := telegram.NewClient(appId, apiHash, telegram.Options{
		DCList:         opts.DCList,
		UpdateHandler:  d,
		SessionStorage: sessionStorage,
		Logger:         opts.Logger,
	})

	ctx := context.Background()

	c := &Client{
		Resolver:         opts.Resolver,
		PublicKeys:       opts.PublicKeys,
		DC:               opts.DC,
		DCList:           opts.DCList,
		DisableCopyright: opts.DisableCopyright,
		Session:          opts.Session,
		Dispatcher:       d,
		Client:           client,
		clientType:       cType,
		ctx:              ctx,
		started:          make(chan int),
	}

	c.printCredit()

	go func(c *Client) {
		c.err = client.Run(ctx, c.initialize)
	}(c)
	// wait till client starts
	<-c.started
	return c, nil
}

func (c *Client) login() error {
	authClient := c.Auth()

	if c.clientType.BotToken == "" {
		authFlow := auth.NewFlow(termAuth{
			phone:  c.clientType.Phone,
			client: authClient,
		},
			auth.SendCodeOptions{})
		if err := IfAuthNecessary(authClient, c.ctx, Flow(authFlow)); err != nil {
			return err
		}
	} else {
		status, err := authClient.Status(c.ctx)
		if err != nil {
			return errors.Wrap(err, "auth status")
		}
		if !status.Authorized {
			if _, err := c.Auth().Bot(c.ctx, c.clientType.BotToken); err != nil {
				return errors.Wrap(err, "login")
			}
		}
	}
	return nil
}

func (ch *Client) printCredit() {
	if !ch.DisableCopyright {
		fmt.Printf(`
GoTGProto %s, Copyright (C) 2023 Anony <github.com/anonyindian>
Licensed under the terms of GNU General Public License v3

`, VERSION)
	}
}

func (c *Client) initialize(ctx context.Context) error {
	err := c.login()
	if err != nil {
		return err
	}

	self, err := c.Client.Self(ctx)
	if err != nil {
		return err
	}
	c.Self = self

	c.Dispatcher.Initialize(ctx, c.Client, self)

	if c.Session.GetName() == "" {
		storage.Load("new.session")
	}
	// notify channel that client is up
	close(c.started)

	<-c.ctx.Done()
	return c.ctx.Err()
}

func (c *Client) Idle() error {
	<-c.ctx.Done()
	return c.err
}
