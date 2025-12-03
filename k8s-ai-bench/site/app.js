const DATA_URL = "combined_results.jsonl";
const MODEL_KEYWORDS = {
    "gemini": "Proprietary"
};

async function loadData() {
    try {
        const response = await fetch(DATA_URL);
        if (!response.ok) throw new Error("Failed to load data file: " + DATA_URL);
        const text = await response.text();
        const rawData = text.trim().split('\n').map(line => {
            try { return JSON.parse(line); } catch (e) { return null; }
        }).filter(x => x); 
        processData(rawData);
        if (window.renderPage) window.renderPage();
    } catch (err) {
        console.error(err);
        const container = document.querySelector('.container');
        if(container) {
            container.innerHTML = 
                '<div style="color:#cf222e; text-align:center; margin-top:50px; background:#fff; padding:2rem; border-radius:6px; border:1px solid #e1e4e8;">' +
                '<h2>Error Loading Data</h2>' +
                '<p>Could not fetch <code>'+DATA_URL+'</code>.</p>' +
                '<p style="color:#57606a;"><strong>Note:</strong> If opening locally, you must run a local server (browsers block file:// access).</p>' +
                '<code style="background:#f6f8fa; padding:5px; border-radius:4px;">python3 -m http.server</code>' +
                '</div>';
        }
    }
}

function getModelType(name) {
    const n = name.toLowerCase();
    for (let k in MODEL_KEYWORDS) {
        if (n.includes(k)) return MODEL_KEYWORDS[k];
    }
    return 'Open Source';
}

function passAtK(n, c, k) {
    if (n === 0) return 0.0;
    const p = c / n;
    return (1.0 - Math.pow(1.0 - p, k)) * 100;
}

function processData(rawData) {
    const cleanedData = rawData.map(item => {
        let res = (item.result || 'fail').toString().toLowerCase();
        let msg = null;
        if (res !== 'success' && item.failures && item.failures.length > 0) {
            msg = item.failures[0].message ? item.failures[0].message.trim() : '';
        }
        return {
            model: item.llmConfig?.model || 'Unknown',
            task: item.name || 'Unknown',
            result: res,
            message: msg
        };
    });

    const grouped = {}; 
    const allTasks = new Set();
    
    cleanedData.forEach(item => {
        const m = item.model;
        const t = item.task;
        allTasks.add(t);
        if (!grouped[m]) grouped[m] = {};
        if (!grouped[m][t]) grouped[m][t] = [];
        grouped[m][t].push(item);
    });

    const leaderboard = [];
    const model_details = {};

    for (const model in grouped) {
        const tasksMap = grouped[model];
        const p1s = [];
        const p5s = [];
        let passAllCount = 0;
        let totalRuns = 0;
        const mRows = [];

        for (const tName in tasksMap) {
            const items = tasksMap[tName];
            const n = items.length;
            const c = items.filter(i => i.result === 'success').length;
            totalRuns += n;
            p1s.push(passAtK(n, c, 1));
            p5s.push(passAtK(n, c, 5));
            if (n > 0 && c === n) passAllCount++;

            items.forEach((item, idx) => {
                mRows.push({
                    task: tName,
                    result: item.result,
                    run: idx + 1,
                    message: item.message
                });
            });
        }

        const avgP1 = p1s.length ? p1s.reduce((a,b)=>a+b,0)/p1s.length : 0;
        const avgP5 = p5s.length ? p5s.reduce((a,b)=>a+b,0)/p5s.length : 0;
        const taskCount = Object.keys(tasksMap).length;
        const pAll = taskCount ? (passAllCount / taskCount) * 100 : 0;

        leaderboard.push({
            id: model,
            type: getModelType(model),
            p1: parseFloat(avgP1.toFixed(1)),
            p5: parseFloat(avgP5.toFixed(1)),
            pAll: parseFloat(pAll.toFixed(1)),
            runs: totalRuns,
            tasks: taskCount
        });
        
        mRows.sort((a,b) => (a.task > b.task) ? 1 : (a.task === b.task) ? a.run - b.run : -1);
        model_details[model] = mRows;
    }

    const tasks = [];
    const task_details = {};

    allTasks.forEach(tName => {
        let allRes = [];
        for (const m in grouped) {
            if (grouped[m][tName]) {
                allRes = allRes.concat(grouped[m][tName].map(i => i.result));
            }
        }
        const nTotal = allRes.length;
        const cTotal = allRes.filter(r => r === 'success').length;
        
        tasks.push({
            name: tName,
            p1: parseFloat(passAtK(nTotal, cTotal, 1).toFixed(1)),
            count: nTotal
        });

        const breakdown = [];
        for (const m in grouped) {
            if (grouped[m][tName]) {
                const items = grouped[m][tName];
                const n = items.length;
                const c = items.filter(i => i.result === 'success').length;
                const p1 = passAtK(n, c, 1);
                const runs = items.map((i, idx) => ({ r: idx+1, val: i.result === 'success' ? 'S' : 'F' }));
                breakdown.push({ model: m, p1: parseFloat(p1.toFixed(1)), runs: runs });
            }
        }
        breakdown.sort((a,b) => b.p1 - a.p1);
        task_details[tName] = breakdown;
    });

    leaderboard.sort((a,b) => b.p5 - a.p5);
    tasks.sort((a,b) => a.p1 - b.p1);

    window.PROCESSED_DATA = { leaderboard, tasks, details: model_details, task_details };
}

function getHue(percentage) { return (percentage / 100) * 120; }

function createMiniBar(val, hue) {
    return '<div style="height: 6px; width: 100%; background: #eee; border-radius: 3px; margin-top: 5px; overflow: hidden;"><div style="height: 100%; width: '+val+'%; background-color: hsla('+hue+', 85%, 40%, 1.0);"></div></div>';
}

function createBar(val, hue) {
    return '<div class="score-bar-wrapper"><div class="bar-segment" style="width: '+val+'%; background-color: hsla('+hue+', 85%, 40%, 1.0);"></div></div>';
}

function sortTable(table, colIndex) {
    const tbody = table.querySelector('tbody');
    const rows = Array.from(tbody.querySelectorAll('tr'));
    const header = table.querySelector('th[data-idx=\"'+colIndex+'\"]');
    const isAsc = header.classList.contains('asc');
    const dir = isAsc ? -1 : 1;
    rows.sort((a, b) => {
        const aTxt = a.children[colIndex].innerText.trim();
        const bTxt = b.children[colIndex].innerText.trim();
        const aNum = parseFloat(aTxt.replace(/[^0-9.-]+/g,""));
        const bNum = parseFloat(bTxt.replace(/[^0-9.-]+/g,""));
        if (!isNaN(aNum) && !isNaN(bNum) && (aTxt.includes('%') || aTxt.match(/^\\d/))) return (aNum - bNum) * dir;
        return aTxt.localeCompare(bTxt, undefined, {numeric: true}) * dir;
    });
    tbody.innerHTML = '';
    tbody.append(...rows);
    table.querySelectorAll('th').forEach(th => th.classList.remove('asc', 'desc'));
    header.classList.toggle('asc', !isAsc);
    header.classList.toggle('desc', isAsc);
}

document.addEventListener('DOMContentLoaded', () => {
    loadData();
    document.querySelectorAll('th[data-idx]').forEach(th => {
        th.addEventListener('click', () => sortTable(th.closest('table'), th.dataset.idx));
    });
});
