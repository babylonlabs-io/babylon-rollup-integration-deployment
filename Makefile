start-deployment-btc-staking-integration-bitcoind:
	@$(MAKE) -C $(CURDIR)/deployments/btc-staking-integration-bitcoind \
		start-deployment-btc-staking-integration-bitcoind

start-deployment-btc-staking-integration-bitcoind-demo:
	@$(MAKE) -C $(CURDIR)/deployments/btc-staking-integration-bitcoind \
		NUM_VALIDATORS=${NUM_VALIDATORS} \
		start-deployment-btc-staking-integration-bitcoind-demo

run-go-demo:
	@$(MAKE) -C $(CURDIR)/deployments/btc-staking-integration-bitcoind \
		run-go-demo

stop-deployment-btc-staking-integration-bitcoind:
	@$(MAKE) -C $(CURDIR)/deployments/btc-staking-integration-bitcoind \
		stop-deployment-btc-staking-integration-bitcoind
