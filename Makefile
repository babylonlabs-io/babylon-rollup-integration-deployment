run-cosmos-bsn-demo:
	@$(MAKE) -C $(CURDIR)/deployments/cosmos-bsn-demo \
		run-demo

run-rollup-bsn-demo:
	@$(MAKE) -C $(CURDIR)/deployments/rollup-bsn-demo \
		run-demo

stop-cosmos-bsn-demo:
	@$(MAKE) -C $(CURDIR)/deployments/cosmos-bsn-demo \
		stop-deployment

stop-rollup-bsn-demo:
	@$(MAKE) -C $(CURDIR)/deployments/rollup-bsn-demo \
		stop-deployment

stop-deployment: stop-cosmos-bsn-demo stop-rollup-bsn-demo
