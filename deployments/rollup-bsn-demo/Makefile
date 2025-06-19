DOCKER := $(shell which docker)
GIT_TOPLEVEL := $(shell git rev-parse --show-toplevel)

build-bitcoindsim:
	$(MAKE) -C $(GIT_TOPLEVEL)/contrib/images bitcoindsim

build-babylond:
	$(MAKE) -C $(GIT_TOPLEVEL)/babylon/contrib/images babylond

build-vigilante:
	$(MAKE) -C $(GIT_TOPLEVEL)/vigilante build-docker

build-btc-staker:
	$(MAKE) -C $(GIT_TOPLEVEL)/btc-staker build-docker

build-finality-provider:
	$(MAKE) -C $(GIT_TOPLEVEL)/finality-provider build-docker

build-covenant-emulator:
	$(MAKE) -C $(GIT_TOPLEVEL)/covenant-emulator build-docker

build-covenant-signer:
	$(MAKE) -C $(GIT_TOPLEVEL)/covenant-emulator/covenant-signer build-docker

build-tmkms:
	$(MAKE) -C $(GIT_TOPLEVEL)/contrib/images tmkms

build-deployment: build-babylond build-bitcoindsim build-vigilante build-btc-staker build-finality-provider build-covenant-signer build-covenant-emulator build-tmkms

start-deployment: stop-deployment build-deployment
	./pre-deployment.sh
	$(DOCKER) compose -f artifacts/docker-compose.yml up -d
	./post-deployment.sh

stop-deployment:
	$(DOCKER) compose -f artifacts/docker-compose.yml down
	rm -rf $(CURDIR)/.testnets

run-demo: start-deployment
	@echo "🚀 Running Rollup BSN Demo..."
	./demo.sh
