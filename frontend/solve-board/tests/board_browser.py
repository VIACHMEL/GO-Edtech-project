"""Run with Python Playwright after building the board bundle."""

import json
import os
import re
import struct
import zlib
from pathlib import Path
from urllib.parse import urlparse

from playwright.sync_api import expect, sync_playwright

ORIGIN = "http://127.0.0.1:8099"
STATIC_ROOT = Path(__file__).resolve().parents[2] / "static"
STATUS = "#solve-board-status"


def png_image():
    def chunk(name, data):
        return struct.pack(">I", len(data)) + name + data + struct.pack(">I", zlib.crc32(name + data))

    header = struct.pack(">IIBBBBB", 80, 40, 8, 6, 0, 0, 0)
    scanline = b"\0" + b"\xd0\x10\x30\xff" * 80
    return b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", header) + chunk(b"IDAT", zlib.compress(scanline * 40)) + chunk(b"IEND", b"")


class BoardFixture:
    def __init__(self):
        self.files = {}
        self.connections = {}
        self.elements = {}
        self.seq = 0
        self.block_uploads = False
        self.downloads = []

    def attach(self, page):
        page.route(ORIGIN + "/**", self.http)
        page.route_web_socket(re.compile(r".*/ws/boards/12$"), lambda conn: self.socket(page, conn))

    def http(self, route):
        path = urlparse(route.request.url).path
        if path == "/solve/":
            route.fulfill(content_type="text/html; charset=utf-8", body='''<!doctype html>
                <meta name="viewport" content="width=device-width, initial-scale=1">
                <link rel="stylesheet" href="/static/styles.css">
                <div id="solve-board" data-current-index="0"></div>
                <div id="solve-board-navigation-status"></div>
                <button data-task-index="1">Второе задание</button>
                <script type="application/json" id="variant-data">
                    {"id":9,"tasks":[{}, {"board":{"id":12,"ws_path":"/ws/boards/12"}}]}
                </script>
                <script>
                    document.querySelector('[data-task-index="1"]').addEventListener("click", () => {
                        const board = document.getElementById("solve-board");
                        board.dataset.currentIndex = "1";
                        board.dispatchEvent(new Event("solve:task-changed"));
                    });
                </script>
                <script type="module" src="/static/solve-board/dist/solve-board.js"></script>''')
        elif path.startswith("/static/"):
            route.fulfill(path=STATIC_ROOT / path.removeprefix("/static/"))
        elif path.startswith("/api/v1/boards/12/files/"):
            file_id = path.rsplit("/", 1)[1]
            if route.request.method == "PUT":
                if self.block_uploads:
                    route.abort()
                    return
                self.files[file_id] = route.request.post_data_buffer
                route.fulfill(status=204)
            else:
                self.downloads.append(file_id)
                if file_id not in self.files:
                    route.fulfill(status=404)
                else:
                    route.fulfill(content_type="image/png", body=self.files[file_id])
        else:
            route.fulfill(status=404)

    def socket(self, page, conn):
        self.connections[page] = conn
        conn.send(json.dumps({"type": "snapshot", "draft_owner": 3, "seq": self.seq,
                              "can_edit": True, "elements": list(self.elements.values())}))

        def receive(raw):
            op = json.loads(raw)
            changed = op["payload"]["elements"]
            assert all(item.get("fileId") in self.files for item in changed if item["type"] == "image")
            self.seq += 1
            self.elements.update((item["id"], item) for item in changed)
            frame = {"type": "delta", "draft_owner": 3, "seq": self.seq, "can_edit": True, "elements": changed}
            for peer in self.connections.values():
                peer.send(json.dumps({**frame, "ack_op_id": op["op_id"]} if peer == conn else frame))

        conn.on_message(receive)


def check_board(browser):
    fixture = BoardFixture()
    sender = browser.new_page(viewport={"width": 1200, "height": 800})
    viewer = browser.new_page(viewport={"width": 820, "height": 1180}, has_touch=True, is_mobile=True)
    viewer.add_init_script("HTMLElement.prototype.requestFullscreen = undefined")
    errors = []
    for page in (sender, viewer):
        page.on("pageerror", lambda error: errors.append(str(error)))
        fixture.attach(page)
        page.goto(ORIGIN + "/solve/")
        page.get_by_role("button", name="Второе задание").click()
        expect(page.locator(STATUS)).to_have_text("Доска синхронизирована")

    sender.locator('input[type="file"]').set_input_files({"name": "invalid.svg", "mimeType": "image/svg+xml", "buffer": b"<svg/>"})
    expect(sender.get_by_text("Выберите изображение PNG или JPEG.")).to_be_visible()
    fixture.block_uploads = True
    png = png_image()
    sender.locator('input[type="file"]').set_input_files({"name": "drawing.png", "mimeType": "image/png", "buffer": png})
    expect(sender.get_by_role("button", name="Повторить сохранение")).to_be_visible()
    assert not fixture.files and not fixture.elements
    fixture.block_uploads = False
    sender.reload()
    sender.get_by_role("button", name="Повторить сохранение").click()
    expect(sender.locator(STATUS)).to_have_text("Доска синхронизирована")
    assert list(fixture.files.values()) == [png]
    file_id = next(iter(fixture.files))
    assert any(element["fileId"] == file_id for element in fixture.elements.values())

    expect(viewer.get_by_role("button", name="SVG")).to_be_enabled()
    with viewer.expect_download() as svg_file:
        viewer.get_by_role("button", name="SVG").tap()
    assert b"data:image/png;base64," in svg_file.value.path().read_bytes()
    assert fixture.downloads == [file_id]
    with viewer.expect_download() as png_file:
        viewer.get_by_role("button", name="PNG").tap()
    assert png_file.value.path().read_bytes().startswith(png[:8])

    viewer.get_by_role("button", name="Увеличить масштаб").tap()
    expect(viewer.locator(".board-zoom output")).to_have_text("125%")
    viewer.get_by_role("button", name="На весь экран").tap()
    expect(viewer.locator(".board-editor")).to_have_attribute("data-expanded", "true")
    expect(viewer.get_by_role("button", name="Свернуть доску")).to_be_visible()
    viewer.keyboard.press("Escape")
    expect(viewer.locator(".board-editor")).to_have_attribute("data-expanded", "false")
    assert not errors, errors


if __name__ == "__main__":
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(channel=os.environ.get("PLAYWRIGHT_CHANNEL"))
        check_board(browser)
        browser.close()
    print("Browser board checks passed")
