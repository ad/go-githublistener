IMG = github.com/ad/go-githublistener
DEV-TAG = dev
TAG = latest
CWD = $(shell pwd)

build: #test 
	@docker build -t $(IMG):$(TAG) .

devbuild: #test 
	@docker build -t $(IMG):$(DEV-TAG) . -f Dockerfile-dev

test:
	@docker run --rm -v $(CWD):$(CWD) -w $(CWD) golang:alpine sh -c "CGO_ENABLED=0 go test -mod=vendor -v"

clean:
	@docker-compose -f docker-compose.dev.yml rm -sfv
	@docker-compose -f docker-compose.yml rm -sfv

dev: devbuild
	@docker-compose -f docker-compose.dev.yml up

up: build
	@docker-compose -f docker-compose.yml up

logs:
	@docker-compose -f docker-compose.dev.yml logs -f

.PHONY: build devbuild test clean dev
