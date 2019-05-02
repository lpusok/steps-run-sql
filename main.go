package main

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"os"
	"path"

	"github.com/bitrise-io/go-utils/command/git"
	"github.com/bitrise-io/go-utils/errorutil"
	"github.com/bitrise-io/go-utils/log"
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
	log.Printf("cloning into: %s\n", dir)

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

func connectToDB() (*sql.DB, error) {
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)
	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		return nil, err
	}

	err = db.Ping()
	if err != nil {
		return nil, err
	}
	log.Printf("Successfully connected!")
	return db, nil
}

func runSQLStatement(db *sql.DB, statement string) error {
	_, err := db.Exec(statement)
	if err != nil {
		return err
	}
	return nil
}

func testQuery(db *sql.DB) error {
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
	err := row.Scan(&table.schemaname, &table.tablename, &table.tableowner, &table.tablespace,
		&table.hasindexes, &table.hasrules, &table.hastriggers, &table.rowsecurity)
	switch err {
	case sql.ErrNoRows:
		log.Printf("No rows were returned!")
		return nil
	case nil:
		log.Printf("%v", table)
	default:
		return err
	}
	return nil
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
	db, err := connectToDB()
	if err != nil {
		panic(err)
	}
	defer func() {
		err := db.Close()
		if err != nil {
			log.Warnf("failed to close DB")
		}
	}()

	err = testQuery(db)
	if err != nil {
		log.Warnf("failed to list tables, error: %s", err)
	}

	for _, script := range scripts {
		file, err := os.Open(script)
		if err != nil {
			panic(fmt.Errorf("failed to open file, error: %s", err))
		}
		sqlStatement, err := ioutil.ReadAll(file)
		if err != nil {
			panic(fmt.Errorf("failed to read file content, error: %s", err))
		}
		err = runSQLStatement(db, string(sqlStatement))
		if err != nil {
			log.Warnf("failed to run script: %s, error: %s", script, err)
		}
	}
}
