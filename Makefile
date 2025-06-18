start-deployment:
	@$(MAKE) -C $(CURDIR)/deployments/bsn-integration-demos \
		start-deployment

run-cosmos-demo:
	@$(MAKE) -C $(CURDIR)/deployments/bsn-integration-demos \
		run-cosmos-demo

run-rollup-demo:
	@$(MAKE) -C $(CURDIR)/deployments/bsn-integration-demos \
		run-rollup-demo

stop-deployment:
	@$(MAKE) -C $(CURDIR)/deployments/bsn-integration-demos \
		stop-deployment
