.PHONY: config1 config2 build \
		prune clean-containers down up rebuild clean all \
		orderservice order \
		kitchenworker1 kitchenworker2 kitchenworker3 worker1 worker2 worker3 \
		trackingservice tracking \
		notificationservice notification

# ----------------- CONFIGURATION -------------------

# Set the config
config1:
	git config user.email "amirkhan.ibragimov@nu.edu.kz"
	git config user.name "Amirkhan"

config2:
	git config user.email "a.kuanyshev11@gmail.com"
	git config user.name "Ayan"

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
	docker-compose down -v

# Start the stack normally
up:
	docker-compose up

# Build fresh images and start
rebuild:
	docker-compose up --build

# Convenience bundle:  clean containers, volumes, and images
clean: clean-containers down prune 

# Convenience bundle: clean everything and rebuild
all: clean-containers down prune rebuild


# ----------------- RUN COMMANDS -------------------
orderservice order:
	go build -o restaurant-system .
	./restaurant-system --mode=order-service --port=3000 2>&1 | jq .

kitchenworker1 worker1:
	./restaurant-system --mode=kitchen-worker --worker-name="chef_anna" --prefetch=1 2>&1 | jq . &

kitchenworker2 worker2:
	./restaurant-system --mode=kitchen-worker --worker-name="chef_mario" --order-types="dine_in" 2>&1 | jq . &

kitchenworker3 worker3:
	./restaurant-system --mode=kitchen-worker --worker-name="chef_luigi" --order-types="delivery" 2>&1 | jq . &

trackingservice tracking:
	./restaurant-system --mode=tracking-service --port=3002 2>&1 | jq .

notificationservice notification:
	./restaurant-system --mode=notification-subscriber 2>&1 | jq .

