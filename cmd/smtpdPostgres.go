package main

// Run and SMTP server with a postgres storage backend

import (
	"bytes"
	"database/sql"
	"flag"
	"fmt"
	"io/ioutil"

	"github.com/reds/smtpd"

	_ "github.com/lib/pq"
)

type storage struct {
	db *sql.DB
}

func (db *storage) Save(to string, from []string, data *bytes.Buffer) error {
	buf, err := ioutil.ReadAll(data)
	if err != nil {
		return err
	}
	fmt.Println("db:", from)
	fmt.Println("db:", to)
	fmt.Println("db:", string(buf))
	return fmt.Errorf("db unimplemented")
}

func main() {
	// no config file yet, all command line args
	hp := flag.String("hp", ":25", "Server listening Host and Port")
	tlsCert := flag.String("cert", "", "PEM file containing server certificate")
	tlsPriv := flag.String("priv", "", "PEM file containing server private key")
	dbUser := flag.String("dbu", "postgres", "DB username")
	dbPw := flag.String("dbpw", "", "DB password")
	dbHost := flag.String("dbh", "localhost", "DB host")
	dbPort := flag.String("dbp", "5432", "DB port")
	dbTable := flag.String("dbt", "email", "DB table")
	flag.Parse()
	myDomains := flag.Args() // any args are considered domain names

	db, err := dbConnect(*dbUser, *dbPw, *dbHost, *dbPort, *dbTable)
	if err != nil {
		fmt.Println(err)
	}

	err = smtpd.ListenAndServer(
		&smtpd.ServerConfig{HostPort: *hp, MyDomains: myDomains},
		&storage{db: db},
		*tlsCert,
		*tlsPriv)
	if err != nil {
		fmt.Println(err)
	}
}

func dbConnect(user, pw, host, port, dbname string) (*sql.DB, error) {
	pgConnect := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		user,
		pw,
		host,
		port,
		dbname)
	db, err := sql.Open("postgres", pgConnect)
	if err != nil {
		return nil, err
	}
	err = db.Ping()
	if err != nil {
		return nil, err
	}
	return db, nil
}
