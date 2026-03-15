IMAGE ?= mrexz/cibot
TAG   ?= latest

.PHONY: build tag-latest push all

build:
	docker build -t $(IMAGE):$(TAG) .

tag-latest:
	docker tag $(IMAGE):$(TAG) $(IMAGE):latest

push:
	docker push $(IMAGE):$(TAG)

all: build push
