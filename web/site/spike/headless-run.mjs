// Runs a page in headless Chrome until it sets window.spikeDone, then saves a screenshot and prints
// the page's console output and the text of <pre id="results">. No dependencies (Node >= 22 for the
// built-in WebSocket). Software GL via SwiftShader, so WebGL pages render at a few fps.
//
//   node spike/headless-run.mjs <url> <screenshot.png> [timeoutSeconds]
//
// Environment: DONE_EXPR overrides the JS expression that is polled until it is true (default
// 'window.spikeDone === true'); ACTION_EXPR is an expression (may return a promise) evaluated once
// after that, its value is printed; SETTLE_MS is the wait after that before the screenshot (default 0).
//
// Exit code 0 when the page finished and its results contain no 'FAIL' line, 1 otherwise.
import {spawn} from 'node:child_process';
import {writeFileSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';

const [url, shot, timeoutSec = '120'] = process.argv.slice(2);
if (!url || !shot) {
    console.error('usage: node headless-run.mjs <url> <screenshot.png> [timeoutSeconds]');
    process.exit(2);
}

const chrome = spawn(process.env.CHROME || 'google-chrome', [
    '--headless=new', '--remote-debugging-port=0', '--no-first-run',
    '--user-data-dir=' + join(process.env.HEADLESS_PROFILE || tmpdir(), 'headless-run-profile'),
    '--use-angle=swiftshader', '--enable-unsafe-swiftshader', '--window-size=1280,800', 'about:blank',
], {stdio: ['ignore', 'ignore', 'pipe']});

const wsUrl = await new Promise((resolve, reject) => {
    let buf = '';
    chrome.stderr.on('data', (d) => {
        buf += d;
        const m = buf.match(/DevTools listening on (ws:\/\/\S+)/);
        if (m) {
            resolve(m[1]);
        }
    });
    chrome.on('exit', (code) => reject(new Error('chrome exited with code ' + code)));
});

const ws = new WebSocket(wsUrl);
await new Promise((resolve, reject) => {
    ws.onopen = resolve;
    ws.onerror = reject;
});
let nextId = 0;
const pending = new Map();
const consoleLines = [];
ws.onmessage = (ev) => {
    const msg = JSON.parse(ev.data);
    if (msg.id !== undefined && pending.has(msg.id)) {
        pending.get(msg.id)(msg);
        pending.delete(msg.id);
    } else if (msg.method === 'Runtime.consoleAPICalled') {
        consoleLines.push(msg.params.args.map((a) => a.value ?? a.description ?? '').join(' '));
    } else if (msg.method === 'Runtime.exceptionThrown') {
        const d = msg.params.exceptionDetails;
        consoleLines.push('EXCEPTION ' + (d.exception?.description || d.text));
    }
};

function send(method, params = {}, sessionId = undefined) {
    return new Promise((resolve, reject) => {
        const id = ++nextId;
        pending.set(id, (m) => m.error ? reject(new Error(method + ': ' + JSON.stringify(m.error))) : resolve(m.result));
        ws.send(JSON.stringify({id, method, params, sessionId}));
    });
}

let done = false;
let results = '';
try {
    const {targetId} = await send('Target.createTarget', {url: 'about:blank'});
    const {sessionId} = await send('Target.attachToTarget', {targetId, flatten: true});
    await send('Runtime.enable', {}, sessionId);
    await send('Page.enable', {}, sessionId);
    await send('Page.navigate', {url}, sessionId);
    const deadline = Date.now() + Number(timeoutSec) * 1000;
    while (Date.now() < deadline) {
        const r = await send('Runtime.evaluate', {expression: process.env.DONE_EXPR || 'window.spikeDone === true', returnByValue: true}, sessionId);
        if (r.result.value === true) {
            done = true;
            break;
        }
        await new Promise((resolve) => setTimeout(resolve, 250));
    }
    if (done && process.env.ACTION_EXPR) {
        const a = await send('Runtime.evaluate', {expression: process.env.ACTION_EXPR, awaitPromise: true, returnByValue: true}, sessionId);
        consoleLines.push('---- action result: ' + JSON.stringify(a.exceptionDetails ? a.exceptionDetails : a.result.value));
    }
    await new Promise((resolve) => setTimeout(resolve, Number(process.env.SETTLE_MS || 0)));
    const r = await send('Runtime.evaluate', {
        expression: "(document.getElementById('results') || {}).textContent || ''", returnByValue: true,
    }, sessionId);
    results = r.result.value;
    const png = await send('Page.captureScreenshot', {format: 'png'}, sessionId);
    writeFileSync(shot, Buffer.from(png.data, 'base64'));
    await send('Browser.close');
} finally {
    chrome.kill();
}
console.log(consoleLines.join('\n'));
console.log('---- results');
console.log(results);
console.log(done ? 'DONE' : 'TIMEOUT');
process.exit(done && !results.includes('FAIL') ? 0 : 1);
