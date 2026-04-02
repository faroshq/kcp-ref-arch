.PHONY: build install clean docker-build

PLUGINS := platform-shell platform-compute platform-kubernetes

install:
	@for p in $(PLUGINS); do \
		echo "Installing $$p..."; \
		cd plugins/$$p && npm install && cd ../..; \
	done

build: install
	@for p in $(PLUGINS); do \
		echo "Building $$p..."; \
		cd plugins/$$p && npm run build && cd ../..; \
	done

clean:
	@for p in $(PLUGINS); do \
		rm -rf plugins/$$p/dist plugins/$$p/node_modules; \
	done

docker-build:
	docker build -t platform-console:latest .
