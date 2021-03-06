package main

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/pathutil"
	"github.com/bitrise-tools/go-steputils/stepconf"
	pg_query "github.com/lfittl/pg_query_go"
	_ "github.com/lib/pq"
	"github.com/olekukonko/tablewriter"
)

type dbInfo struct {
	host         string
	port         int
	username     string
	password     string
	databaseName string
	sslmode      string
}

func connectToDB(dbInfo dbInfo) (*sql.DB, error) {
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=%s",
		dbInfo.host, dbInfo.port, dbInfo.username,
		dbInfo.password, dbInfo.databaseName, dbInfo.sslmode)
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
	rows, err := db.Query(statement)
	if err != nil {
		return fmt.Errorf("failed to query statement, error: %s", err)
	}

	for {
		columnTypes, err := rows.ColumnTypes()
		if err != nil {
			return fmt.Errorf("failed to get column types, error: %s", err)
		}
		types := []string{}
		for _, cType := range columnTypes {
			types = append(types, cType.Name())
		}

		cols, err := rows.Columns()
		if err != nil {
			return fmt.Errorf("failed to get columns, error :%s", err)
		}

		allResults := [][]string{}

		// Result is your slice string.
		rawResult := make([][]byte, len(cols))
		dest := make([]interface{}, len(cols)) // A temporary interface{} slice
		for i := range rawResult {
			dest[i] = &rawResult[i] // Put pointers to each string in the interface slice
		}

		for rows.Next() {
			err = rows.Scan(dest...)
			if err != nil {
				return fmt.Errorf("failed to scan row, error: %s", err)
			}

			result := make([]string, len(cols))
			for i, raw := range rawResult {
				if raw == nil {
					result[i] = ""
				} else {
					result[i] = string(raw)
				}
			}

			allResults = append(allResults, result)
		}

		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader(types)
		table.AppendBulk(allResults)
		table.Render()
		log.Printf("Rows: %d", len(allResults))

		if !rows.NextResultSet() {
			break
		}
	}
	return nil
}

type config struct {
	DbHost     string          `env:"db_host,required"`
	DbPort     int             `env:"db_port,required"`
	DbUsername string          `env:"db_username,required"`
	DbPassword stepconf.Secret `env:"db_password"`
	DbName     string          `env:"db_name,required"`
	DbSSLmode  string          `env:"db_sslmode,required"`
	ScriptsDir string          `env:"scripts_dir,dir"`
}

func main() {
	var cfg config
	if err := stepconf.Parse(&cfg); err != nil {
		panic(fmt.Errorf("could not create config: %s", err))
	}
	stepconf.Print(cfg)

	scripsDir, err := pathutil.AbsPath(cfg.ScriptsDir)
	if err != nil {
		panic(fmt.Errorf("failed to convert to absolute dir, error: %s", err))
	}

	entries, err := ioutil.ReadDir(scripsDir)
	if err != nil {
		panic(err)
	}
	scriptFiles := []string{}
	for _, entry := range entries {
		if !entry.IsDir() && path.Ext(strings.ToLower(entry.Name())) == ".sql" {
			scriptFiles = append(scriptFiles, path.Join(scripsDir, entry.Name()))
		}
	}
	log.Printf("Script files: %s", scriptFiles)

	// Connect to DB
	db, err := connectToDB(dbInfo{
		host:         cfg.DbHost,
		port:         cfg.DbPort,
		username:     cfg.DbUsername,
		password:     string(cfg.DbPassword),
		sslmode:      cfg.DbSSLmode,
		databaseName: cfg.DbName,
	})
	if err != nil {
		panic(err)
	}
	defer func() {
		err := db.Close()
		if err != nil {
			log.Warnf("failed to close DB")
		}
	}()

	type script struct {
		path    string
		content string
	}

	// Read script contents
	scripts := make([]script, len(scriptFiles))
	for i, path := range scriptFiles {
		file, err := os.Open(path)
		if err != nil {
			panic(fmt.Errorf("failed to open file: %s, error: %s", path, err))
		}

		contents, err := ioutil.ReadAll(file)
		if err != nil {
			panic(fmt.Errorf("failed to read content, file: %s, error: %s", path, err))
		}

		scripts[i] = script{
			path:    path,
			content: string(contents),
		}
	}

	// Validate queries
	for _, script := range scripts {
		tree, err := pg_query.ParseToJSON(script.content)
		if err != nil {
			panic(fmt.Errorf("failed to validate script, error: %s, path: %s, content: %s", err, script.path, script.content))
		}
		log.Debugf("%s\n", tree)
	}

	// Execute queries
	failure := false
	for _, script := range scripts {
		fmt.Println()
		log.Infof("Preparing to run script: %s", path.Base(script.path))
		log.Printf("Script content:\n%s", script.content)

		err = runSQLStatement(db, script.content)
		if err != nil {
			failure = true
			log.Warnf("failed to execute, error: %s", err)
		}

		log.Infof("Done with script: %s", path.Base(script.path))
	}
	if failure {
		panic("One or more scripts failed.")
	}
}
