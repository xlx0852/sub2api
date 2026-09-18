.PHONY: test build build-headless build-app build-all clean

test:
	CGO_ENABLED=0 go test -tags headless ./...

build:
	mkdir -p dist
	CGO_ENABLED=1 go build -ldflags='-s -w' -o dist/sub2api-tray ./cmd/sub2api-tray

build-headless:
	mkdir -p dist
	CGO_ENABLED=0 go build -tags headless -ldflags='-s -w' -o dist/sub2api-tray-headless ./cmd/sub2api-tray

# macOS .app bundle so the panel is a real Dock/window app (not Safari).
build-app: build
	@mkdir -p "dist/Sub2API Tray.app/Contents/MacOS" "dist/Sub2API Tray.app/Contents/Resources"
	@cp dist/sub2api-tray "dist/Sub2API Tray.app/Contents/MacOS/sub2api-tray"
	@printf '%s\n' \
	'<?xml version="1.0" encoding="UTF-8"?>' \
	'<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">' \
	'<plist version="1.0"><dict>' \
	'<key>CFBundleName</key><string>Sub2API Tray</string>' \
	'<key>CFBundleDisplayName</key><string>Sub2API Tray</string>' \
	'<key>CFBundleIdentifier</key><string>com.sub2api.tray</string>' \
	'<key>CFBundleVersion</key><string>0.1.0</string>' \
	'<key>CFBundleShortVersionString</key><string>0.1.0</string>' \
	'<key>CFBundleExecutable</key><string>sub2api-tray</string>' \
	'<key>CFBundlePackageType</key><string>APPL</string>' \
	'<key>LSMinimumSystemVersion</key><string>12.0</string>' \
	'<key>NSHighResolutionCapable</key><true/>' \
	'<key>LSUIElement</key><false/>' \
	'<key>NSAppTransportSecurity</key><dict><key>NSAllowsLocalNetworking</key><true/></dict>' \
	'</dict></plist>' > "dist/Sub2API Tray.app/Contents/Info.plist"
	@echo "built: dist/Sub2API Tray.app"

build-all: build build-headless build-app
	ls -lh dist

clean:
	rm -rf dist
