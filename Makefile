.PHONY: all build docker

build:
	go get -d ./.
	go build -o bin/bladerf-device-plugin 
docker:
	docker build -t apurer/bladerf-device-plugin .

kubernetes:
	kubectl apply -f bladerf-daemonset.yaml
