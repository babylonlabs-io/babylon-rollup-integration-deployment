start-deployment:
	@$(MAKE) -C $(CURDIR)/deployments/rollup-btc-staking-demo \
		start-deployment

run-demo:
	@$(MAKE) -C $(CURDIR)/deployments/rollup-btc-staking-demo \
		run-demo

stop-deployment:
	@$(MAKE) -C $(CURDIR)/deployments/rollup-btc-staking-demo \
		stop-deployment
