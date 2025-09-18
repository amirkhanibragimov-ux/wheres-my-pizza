.PHONY: config1 config2 build \
		prune clean-containers down up rebuild clean all \
		orderservice

# ----------------- CONFIGURATION -------------------

# Set the config
config1:
	git config --global user.email "amirkhan.ibragimov@nu.edu.kz"
	git config --global user.name "Amirkhan"

config2:
	git config --global user.email "a.kuanyshev11@gmail.com"
	git config --global user.name "Ayan"

# Build the Go binary
build:
	go build -o restaurant-system .


# ----------------- DOCKER COMMANDS -------------------

# Delete all untagged dangling images
prune:
	docker image prune -f

# Force-remove all running/stopped containers
clean-containers:
	-docker ps -aq | xargs docker rm -f

# Stop and remove containers, networks, and volumes defined in docker-compose.yml
down:
	docker compose down -v

# Start the stack normally
up:
	docker compose up

# Build fresh images and start
rebuild:
	docker compose up --build

# Convenience bundle:  clean containers, volumes, and images
clean: prune clean-containers down

# Convenience bundle: clean everything and rebuild
all: prune clean-containers down rebuild


# ----------------- RUN COMMANDS -------------------
orderservice:
	go build -o restaurant-system .
	./restaurant-system --mode=order-service --port=3000 2>&1 | jq .
