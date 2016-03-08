package smtpd

// rfc5321 server

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"strings"
)

type Storage interface {
	Save(from string, to []string, data *bytes.Buffer) error
}

type opHandler func(*context, string) error

var clientCmds = map[string]opHandler{}

func handle(op string, oph opHandler) {
	clientCmds[op] = oph
}

type context struct {
	c       net.Conn // io.ReadWriteCloser
	scanner *bufio.Scanner
	state   sessionState
	from    string
	to      []string
	data    bytes.Buffer
	tls     bool
}

func (ctx *context) reset() {
	ctx.state = sstateInit
	ctx.from = ""
	ctx.to = nil
	ctx.data.Reset()
}

func (ctx *context) send(l string) error {
	fmt.Println("sending:", l)
	_, err := fmt.Fprint(ctx.c, l, "\r\n")
	return err
}

type sessionState int

const (
	sstateInit sessionState = iota
	sstateGotMail
	sstateGotRcpt
	sstateReadyForData
	sstateDataDone
	sstateBadSequence
)

type ServerConfig struct {
	HostPort  string
	Greeting  string
	Hostname  string
	MyDomains []string // domains I will accept email for
}

// hostname, greeting
// extensions: starttls, maxsize

func registerHandlers(cfg *ServerConfig, db Storage, serverCerts []tls.Certificate) {
	handle("helo", func(ctx *context, arg string) error {
		// 250, 504
		ctx.reset()
		return ctx.send("250 redmond5.com")
	})
	handle("ehlo", func(ctx *context, arg string) error {
		// 250, 504
		ctx.reset()
		err := ctx.send("250-redmond5.com")
		if err != nil {
			return err
		}
		if !ctx.tls {
			err = ctx.send("250-STARTTLS")
			if err != nil {
				return err
			}
		}
		// SIZE
		return ctx.send("250 SIZE 35882577") // from gmail
	})

	handle("mail", func(ctx *context, arg string) error {
		// MAIL FROM:<reverse-path> [SP <mail-parameters> ] <CRLF>
		// 250
		// 552, 451, 452, 550, 553, 503, 455, 555
		ctx.from = extractMailbox(arg)
		if !cfg.fromOk(ctx.from) {
			return ctx.send(fmt.Sprint("550 bad reverse-path: ", arg))
		}
		switch ctx.state {
		case sstateGotRcpt:
			ctx.state = sstateReadyForData
		case sstateInit:
			ctx.state = sstateGotMail
		default:
			ctx.state = sstateBadSequence
		}
		return ctx.send(fmt.Sprint("250 sender <", ctx.from, "> ok"))
	})
	handle("rcpt", func(ctx *context, arg string) error {
		// RCPT TO:<forward-path> [ SP <rcpt-parameters> ] <CRLF>
		// 250: address is ok,  251: address ok, but has changed and I will take care of it
		// 550, 551, 552, 553, 450, 451, 452, 503, 455, 555
		to := extractMailbox(arg)
		if !cfg.toOk(to) {
			return ctx.send(fmt.Sprint("550 bad recipient <", to, ">", arg))
		}
		ctx.to = append(ctx.to, to)
		switch ctx.state {
		case sstateGotMail:
			ctx.state = sstateReadyForData
		case sstateInit:
			ctx.state = sstateGotRcpt
		default:
			ctx.state = sstateBadSequence
		}
		return ctx.send(fmt.Sprint("250 recipient <", to, "> ok"))
	})

	handle("data", func(ctx *context, arg string) error {
		// 354: send data
		// 503: command out of sequence
		// 554: no valid recipients
		if ctx.state != sstateReadyForData {
			return ctx.send("503 command out of sequence")
		}
		err := ctx.send("354 Ok send data ending with <CRLF>.<CRLF>")
		if err != nil {
			return err
		}
		s := ctx.scanner
		for s.Scan() {
			fmt.Println("server got data:", s.Text())
			if s.Text() == "." {
				break
			}
			_, err := ctx.data.Write(s.Bytes())
			if err != nil {
				return err
			}
			_, err = ctx.data.Write([]byte("\r\n"))
			if err != nil {
				return err
			}
		}
		if s.Err() != nil {
			return s.Err()
		}
		// 250
		// 552,554,
		// 451: Requested action aborted: error in processing
		// 452
		// 450
		// 550: mailbox not found
		err = db.Save(ctx.from, ctx.to, &ctx.data)
		if err != nil {
			log.Println(err)
			return ctx.send("451 error")
		}
		ctx.state = sstateDataDone
		return ctx.send("250 Thank You")
	})
	handle("rset", func(ctx *context, arg string) error {
		ctx.reset()
		return ctx.send("250 Ok")
	})
	handle("vrfy", func(ctx *context, arg string) error {
		//  250, 251, 252
		// 550, 551, 553, 502, 504
		return fmt.Errorf("not implemented")
	})
	handle("expn", func(ctx *context, arg string) error {
		// 250, 251, 252
		// 550, 551, 553, 502, 504
		return fmt.Errorf("not implemented")
	})
	handle("help", func(ctx *context, arg string) error {
		// 211, 214
		// 502, 504
		return fmt.Errorf("not implemented")
	})
	handle("noop", func(ctx *context, arg string) error {
		return ctx.send("250 Ok")
	})
	handle("quit", func(ctx *context, arg string) error {
		defer ctx.c.Close()
		return ctx.send("221 server closing connection")
	})

	// extensions
	handle("starttls", func(ctx *context, arg string) error {
		if ctx.tls {
			return nil
		}
		err := ctx.send("220 TLS ok")
		if err != nil {
			return err
		}
		sc := tls.Server(ctx.c, &tls.Config{Certificates: serverCerts})
		ctx.reset()
		ctx.c = sc
		ctx.scanner = bufio.NewScanner(sc)

		err = sc.Handshake()
		if err != nil {
			log.Println(err)
			return err
		}
		ctx.tls = true
		return nil
	})
}

func serve(c net.Conn) {
	defer c.Close()
	s := bufio.NewScanner(c)
	_, err := fmt.Fprint(c, "220 redmond5.com ESMTP goMail 0.1\r\n")
	if err != nil {
		log.Println(err)
		return
	}
	ctx := &context{c: c, scanner: s, state: 0}
	for ctx.scanner.Scan() { // gets reset by starttls
		fmt.Println("server got:", ctx.scanner.Text())
		cmd, rest, err := parseLine(ctx.scanner.Text())
		if err != nil {
			log.Println(err)
			return
		}
		err = cmd(ctx, rest)
		if err != nil {
			log.Println(err)
			return
		}
	}
}

func ListenAndServer(scfg *ServerConfig, db Storage, certFile, keyFile string) error {
	var serverCerts = make([]tls.Certificate, 1)
	var err error
	serverCerts[0], err = tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return err
	}
	registerHandlers(scfg, db, serverCerts)
	l, err := net.Listen("tcp", scfg.HostPort)
	if err != nil {
		return err
	}
	for {
		c, err := l.Accept()
		if err != nil {
			log.Println("accept:", err)
		}
		go serve(c)
	}
}

func parseLine(l string) (opHandler, string, error) {
	c := strings.ToLower(strings.ToLower(l))
	for k, v := range clientCmds {
		if strings.HasPrefix(c, k) {
			return v, c[4:], nil
		}
	}
	return nil, "", fmt.Errorf("not implemented: %s", l)
}

func (cfg *ServerConfig) fromOk(from string) bool {
	if from == "" {
		return false
	}
	return true
}

func (cfg *ServerConfig) toOk(to string) bool {
	for _, v := range cfg.MyDomains {
		if strings.HasSuffix(strings.ToLower(to), strings.ToLower(v)) {
			return true
		}
	}
	return false
}

func extractMailbox(in string) string {
	if som := strings.Index(in, "<"); som > 0 {
		if eom := strings.Index(in[som:], ">"); eom > 0 {
			return strings.TrimSpace(in[som+1 : som+eom])
		}
	}
	return ""
}
