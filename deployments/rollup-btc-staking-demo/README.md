# Rollup BTC Staking Demo

This deployment demonstrates the integration of Babylon network with rollup chains using BTC staking and smart contracts. It showcases how rollup chains can leverage Babylon's Bitcoin security through finality contracts.

## Components

1. **Babylon Network**: Two nodes of a private Babylon network providing Bitcoin security.
2. **BTC Regression Testnet**: A local Bitcoin testnet for testing and development.
3. **Babylon Finality Provider**: A finality provider securing the Babylon chain.
4. **BTC Staker**: Service that stakes BTC to Babylon finality providers.
5. **Finality Contract**: Smart contract deployed on Babylon that handles rollup finality.
6. **Crypto Operations Tool**: CLI tool for managing cryptographic operations (key generation, randomness commitment, finality signatures).

## Key Features

- **Smart Contract Integration**: Uses finality contracts instead of separate consumer chain software
- **Batch Processing**: Commits randomness for thousands of blocks, then submits finality signatures in batches
- **Automated Demo**: Complete end-to-end demonstration with retry logic and error handling
- **Rollup Focus**: Designed specifically for rollup chain integration patterns

## Usage

### Start the rollup BTC staking demo

```shell
git submodule update --init
make run-demo
```

This command will:

- Stop any existing deployment
- Build all necessary components (babylond, bitcoindsim, vigilante, btc-staker, finality-provider, covenant-emulator)
- Run the pre-deployment setup
- Start the Docker containers
- Run the post-deployment setup
- Execute the complete rollup BTC staking demo

### Run demo only (assuming deployment is ready)

```shell
make run-demo-only
```

### Stop the deployment

```shell
make stop-deployment
git submodule deinit
```

This will stop and remove the Docker containers, clean up the test network data, and de-initialize submodules.

## Demo Flow

1. **Setup**: Deploy finality contract and register consumer chain
2. **Key Generation**: Create cryptographic keys for finality providers
3. **Finality Provider Creation**: Register finality providers on-chain
4. **BTC Delegation**: Stake BTC to the finality providers
5. **Randomness Commitment**: Commit public randomness for 1000 blocks
6. **Finality Signatures**: Submit finality signatures for 10 blocks in batch
7. **Verification**: Verify all operations succeeded with retry logic
