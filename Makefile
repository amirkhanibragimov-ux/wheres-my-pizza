.PHONY: clean-containers down up rebuild prune all

# Set the config
config1:
	git config --global user.email "amirkhan.ibragimov@nu.edu.kz"
	git config --global user.name "Amirkhan"

config2:
	git config --global user.email "a.kuanyshev11@gmail.com"
	git config --global user.name "Ayan"

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

# Delete all untagged dangling images
prune:
	docker image prune -f

# Convenience: clean everything and rebuild
all: prune clean-containers down rebuild
