.PHONY: all build docker

build:
	go get -d ./.
	go build -o bin/bladerf-device-plugin 
docker:
	docker build -t apurer/bladerf-device-plugin .
	docker tag apurer/bladerf-device-plugin:latest localhost:5000/apurer/bladerf-device-plugin:latest
	docker push localhost:5000/apurer/bladerf-device-plugin:latest
kubernetes:
	kubectl apply -f bladerf-daemonset.yaml
