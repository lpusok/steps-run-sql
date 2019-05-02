package main

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"log"
	"path"

	"github.com/bitrise-io/go-utils/command/git"
	"github.com/bitrise-io/go-utils/errorutil"
	_ "github.com/lib/pq"
)

const (
	host     = "localhost"
	port     = 5432
	user     = "lpusok"
	password = ""
	dbname   = "template1"

	scriptsRepo = "git@github.com:lpusok/hackathon-data-scripts.git"
)

func cloneRepo() (string, error) {
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir")
	}
	fmt.Printf("cloning into: %s\n", dir)

	repo, err := git.New(dir)
	if err != nil {
		return "", fmt.Errorf("failed to create repository")
	}
	command := repo.Clone(scriptsRepo)

	out, err := command.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		if errorutil.IsExitStatusError(err) {
			return "", fmt.Errorf("failed to clone repo, output: %s", out)
		}
		return "", fmt.Errorf("failed to execute clone command, error: %s", err)
	}
	return dir, nil
}

func main() {
	// Clone data scripts repo
	dir, err := cloneRepo()
	if err != nil {
		panic(err)
	}
	scripsDir := path.Join(dir, "scripts")

	entries, err := ioutil.ReadDir(scripsDir)
	if err != nil {
		panic(err)
	}

	scripts := []string{}
	for _, entry := range entries {
		if !entry.IsDir() && path.Ext(entry.Name()) == ".SQL" {
			scripts = append(scripts, path.Join(scripsDir, entry.Name()))
		}
	}

	log.Printf("Scripts to run: %s", scripts)

	// Connect to DB
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)
	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		panic(err)
	}
	fmt.Println("Successfully connected!")

	type Table struct {
		schemaname  string
		tablename   string
		tableowner  string
		tablespace  *string
		hasindexes  bool
		hasrules    bool
		hastriggers bool
		rowsecurity bool
	}
	sqlStatement := `SELECT * FROM pg_catalog.pg_tables;`
	var table Table
	row := db.QueryRow(sqlStatement)
	err = row.Scan(&table.schemaname, &table.schemaname, &table.tableowner, &table.tablespace,
		&table.hasindexes, &table.hasrules, &table.hastriggers, &table.rowsecurity)
	switch err {
	case sql.ErrNoRows:
		fmt.Println("No rows were returned!")
		return
	case nil:
		fmt.Println(table)
	default:
		panic(err)
	}
}
