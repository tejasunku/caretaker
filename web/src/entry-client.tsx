// @refresh reload
import "virtual:uno.css";
import "./global.css";
import { mount, StartClient } from "@solidjs/start/client";

mount(() => <StartClient />, document.getElementById("app")!);
