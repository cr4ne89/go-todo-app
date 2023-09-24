# Go Todo App
TODO管理アプリケーション

[Go言語プログラミングエッセンス](https://gihyo.jp/book/2023/978-4-297-13419-8)に記載されているWebアプリケーション

# Installation
```
$ go install github.com/cr4ne89/go-todo-app@latest
```

# Setup
- install PostgreSQL
- create `.env` and write `export DATABASE_URL=postgresql://user:pass@localhost:5432/dbname?sslmode=disable`
- `source .env`
- `./go-todo-app`
