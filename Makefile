.PHONY: build frontend embed backend clean dev-frontend

frontend:
	cd frontend && pnpm install && pnpm build

embed: frontend
	rm -rf internal/embedui/sites/user
	mkdir -p internal/embedui/sites/user
	cp -r frontend/out/. internal/embedui/sites/user/

backend:
	go build -ldflags "-X main.Version=$$(git describe --tags --always 2>/dev/null || echo dev)" -o CSOJ ./cmd/CSOJ

build: embed backend

dev-frontend:
	cd frontend && pnpm dev

clean:
	rm -rf frontend/out frontend/node_modules CSOJ
