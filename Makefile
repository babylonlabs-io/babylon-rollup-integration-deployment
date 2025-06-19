run-cosmos-demo:
	@$(MAKE) -C $(CURDIR)/deployments/cosmos-bsn-demo \
		run-demo

run-rollup-demo:
	@$(MAKE) -C $(CURDIR)/deployments/rollup-bsn-demo \
		run-demo

stop-cosmos-demo:
	@$(MAKE) -C $(CURDIR)/deployments/cosmos-bsn-demo \
		stop-deployment

stop-rollup-demo:
	@$(MAKE) -C $(CURDIR)/deployments/rollup-bsn-demo \
		stop-deployment

stop-deployment: stop-cosmos-demo stop-rollup-demo
