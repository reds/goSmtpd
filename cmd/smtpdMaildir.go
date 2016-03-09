package main

// Run and SMTP server with a maildir storage backend

import (
	"bytes"
	"flag"
	"fmt"
	"github.com/reds/smtpd"
	"io/ioutil"
	"os"
	"time"
)

type storage struct {
	mailDirRoot string
}

func (db *storage) Save(from string, to []string, data *bytes.Buffer) error {
	/*
		Return-Path: <redmond.martin@gmail.com>
			X-Original-To: jfidjfi@redmond5.com
		Delivered-To: jfidjfi@redmond5.com
		Received: from mail-vk0-f49.google.com (mail-vk0-f49.google.com [209.85.213.49])
		by chat52.com (Postfix) with ESMTPS id 83C902067B
		for <jfidjfi@redmond5.com>; Wed,  9 Mar 2016 01:50:52 +0000 (UTC)
	*/
	var t string
	if len(to) > 0 {
		t = to[0]
	}
	var msg bytes.Buffer
	fmt.Fprintln(&msg, "Return-Path: <"+from+">")
	fmt.Fprintln(&msg, "X-Original-To: <"+t+">")
	fmt.Fprintln(&msg, "Delivered-To: <"+t+">")
	fmt.Fprintln(&msg, "Received: from ... by ... for ...")
	msg.Write(data.Bytes())

	fn := fmt.Sprintf("%d.%s.%s", time.Now().Unix(), "jfidjfid", "chat52")
	err := ioutil.WriteFile(db.mailDirRoot+"/tmp/"+fn, msg.Bytes(), 0740)
	if err != nil {
		return err
	}
	err = os.Rename(db.mailDirRoot+"/tmp/"+fn, db.mailDirRoot+"/new/"+fn)
	if err != nil {
		return err
	}
	return nil
}

func main() {
	// no config file yet, all command line args
	hp := flag.String("hp", ":25", "Server listening Host and Port")
	tlsCert := flag.String("cert", "", "PEM file containing server certificate")
	tlsPriv := flag.String("priv", "", "PEM file containing server private key")
	maildir := flag.String("md", "", "Maildir root")
	flag.Parse()
	myDomains := flag.Args() // any args are considered domain names

	err := smtpd.ListenAndServer(
		&smtpd.ServerConfig{
			HostPort:    *hp,
			MyDomains:   myDomains,
			TLSCertFile: *tlsCert,
			TLSKeyFile:  *tlsPriv,
		},
		&storage{mailDirRoot: *maildir})
	if err != nil {
		fmt.Println(err)
	}
}
