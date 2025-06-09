start-deployment:
	@$(MAKE) -C $(CURDIR)/deployments/btc-staking-integration-bitcoind \
		start-deployment

run-demo:
	@$(MAKE) -C $(CURDIR)/deployments/btc-staking-integration-bitcoind \
		run-demo

stop-deployment:
	@$(MAKE) -C $(CURDIR)/deployments/btc-staking-integration-bitcoind \
		stop-deployment
