package main

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/lib/pq"

	"github.com/labstack/echo/v4"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/extra/bundebug"
)

//go:embed templates
var templates embed.FS

// ORMにbunを利用する
type Todo struct {
	bun.BaseModel `bun:"table:todos,alias:t"`

	ID        int64     `bun:"id,pk,autoincrement"`
	Content   string    `bun:"content,notnull"`
	Done      bool      `bun:"done"`
	Until     time.Time `bun:"until,nullzero"`
	CreatedAt time.Time `bun:",nullzero,notnull"`
	UpdatedAt time.Time `bun:",nullzero"`
	DeletedAt time.Time `bun:",soft_delete,nullzero"`
}

// Responseで利用する値
type Data struct {
	Todos  []Todo
	Errors []error
}

// 期限(Until)をパースするための関数
func customFunc(todo *Todo) func([]string) []error {
	return func(values []string) []error {
		if len(values) == 0 || values[0] == "" {
			return nil
		}
		dt, err := time.Parse("2006-01-02T15:04 MST", values[0]+" JST")
		if err != nil {
			return []error{echo.NewBindingError("until", values[0:1], "failed to decode time", err)}
		}
		todo.Until = dt
		return nil
	}
}

type Template struct {
	templates *template.Template
}

func (t *Template) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

func formatDateTime(d time.Time) string {
	if d.IsZero() {
		return ""
	}
	return d.Format("2006-01-02 15:04")
}

func main() {
	sqldb, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer sqldb.Close()

	db := bun.NewDB(sqldb, pgdialect.New())
	defer db.Close()

	db.AddQueryHook(bundebug.NewQueryHook(
		bundebug.WithVerbose(true),
		bundebug.FromEnv("BUNDEBUG"),
	))

	ctx := context.Background()
	_, err = db.NewCreateTable().Model((*Todo)(nil)).IfNotExists().Exec(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// Webフレームワークにechoを利用する
	e := echo.New()

	e.Renderer = &Template{
		templates: template.Must(template.New("").
			Funcs(template.FuncMap{
				"FormatDateTime": formatDateTime,
			}).ParseFS(templates, "templates/*")),
	}

	// GETリクエストの処理
	e.GET("/", func(c echo.Context) error {
		var todos []Todo
		ctx := context.Background()
		err := db.NewSelect().Model(&todos).Order("created_at").Scan(ctx)
		if err != nil {
			e.Logger.Error(err)
			return c.Render(http.StatusBadRequest, "index", Data{
				Errors: []error{errors.New("cannot get todos")},
			})
		}
		return c.Render(http.StatusOK, "index", Data{Todos: todos})
	})

	// POSTリクエストの処理
	e.POST("/", func(c echo.Context) error {
		// フォームパラメータをフィールドにバインドする
		var todo Todo
		errs := echo.FormFieldBinder(c).
			Int64("id", &todo.ID).
			String("content", &todo.Content).
			Bool("done", &todo.Done).
			CustomFunc("until", customFunc(&todo)).
			BindErrors()
		if errs != nil {
			e.Logger.Error(err)
			return c.Render(http.StatusBadRequest, "index", Data{Errors: errs})
		} else if todo.ID == 0 {
			// IDが0の時は登録とみなす
			ctx := context.Background()
			if todo.Content == "" {
				err = errors.New("Todo not found")
			} else {
				todo.CreatedAt = time.Now()
				_, err = db.NewInsert().Model(&todo).Exec(ctx)
				if err != nil {
					e.Logger.Error(err)
					err = errors.New("cannot update")
				}
			}
		} else if c.FormValue("delete") == "削除" {
			// 削除
			ctx := context.Background()
			_, err = db.NewDelete().Model(&todo).Where("id = ?", todo.ID).Exec(ctx)
		} else if c.FormValue("done") != "" {
			// 更新
			ctx := context.Background()
			var orig Todo
			err = db.NewSelect().Model(&orig).Where("id = ?", todo.ID).Scan(ctx)
			if err == nil {
				orig.Done = todo.Done
				orig.UpdatedAt = time.Now()
				_, err = db.NewUpdate().Model(&orig).Where("id = ?", todo.ID).Exec(ctx)
			}
			if err != nil {
				e.Logger.Error(err)
				err = errors.New("cannot update")
			}
		}
		if err != nil {
			return c.Render(http.StatusBadRequest, "index", Data{Errors: []error{err}})
		}
		return c.Redirect(http.StatusFound, "/")
	})

	// サーバーを起動する
	e.Logger.Fatal(e.Start(":8989"))
}
