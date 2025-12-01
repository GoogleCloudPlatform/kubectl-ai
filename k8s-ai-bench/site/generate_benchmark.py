import json
import os
import re
from collections import defaultdict

# Configuration
INPUT_FILE = 'combined_results.jsonl'
REPO_TASK_URL = 'https://github.com/GoogleCloudPlatform/kubectl-ai/tree/main/k8s-ai-bench/tasks/'
REPO_BASE_URL = 'https://github.com/GoogleCloudPlatform/kubectl-ai/tree/main/k8s-ai-bench'

# Dictionary to classify models. 
# Keys are substrings to look for in the model name (case-insensitive).
# Values are the category ('Hosted' or 'Self-Hosted').
MODEL_KEYWORDS = {
    'gemini': 'Hosted',
}

# --- 1. PYTHON DATA PROCESSING ---

def pass_at_k_naive(n, c, k):
    if n == 0: return 0.0 
    success_prob = float(c) / float(n)
    return 1.0 - (1.0 - success_prob) ** k

def get_model_type(model_name):
    """Categorizes model based on keyword dictionary."""
    name_lower = model_name.lower()
    for keyword, category in MODEL_KEYWORDS.items():
        if keyword in name_lower:
            return category
    return 'Self-Hosted'

def process_data(raw_data):
    """
    Processes raw JSON into a structured dictionary ready for the frontend.
    """
    # 1. Group raw items by Model -> Task to assign Run IDs
    # Structure: { Model: { Task: [item1, item2, ...] } }
    grouped_raw = defaultdict(lambda: defaultdict(list))
    
    for item in raw_data:
        model = item.get('llmConfig', {}).get('model', 'Unknown Model')
        name = item.get('name', 'Unknown Task')
        grouped_raw[model][name].append(item)

    # 2. Build Data Structures
    leaderboard = []
    task_stats_summary = [] # For tasks.html
    task_details_map = defaultdict(list) # For task_detail.html
    model_details_map = defaultdict(list) # For model.html

    # --- LEADERBOARD & MODEL DETAILS ---
    for model, tasks_map in grouped_raw.items():
        p1_scores = []
        p5_scores = []
        pass_all_count = 0 
        total_runs_count = 0
        
        # Prepare Model Detail Rows
        model_rows = []
        
        for t_name, items in tasks_map.items():
            # Calculate stats for this model+task combo
            n = len(items)
            c = sum(1 for i in items if str(i.get('result')).lower() == 'success')
            total_runs_count += n
            
            p1_scores.append(pass_at_k_naive(n, c, 1))
            p5_scores.append(pass_at_k_naive(n, c, 5))
            if n > 0 and c == n: pass_all_count += 1
            
            # Process individual runs for Model Details
            for run_idx, item in enumerate(items):
                run_number = run_idx + 1
                
                raw_res = str(item.get('result', 'fail')).lower()
                fail_msg = None
                if raw_res != 'success':
                    failures = item.get('failures', [])
                    if failures: fail_msg = failures[0].get('message', '').strip()

                
                model_rows.append({
                    "task": t_name,
                    "res": raw_res,
                    "run": run_number,
                    "msg": fail_msg
                })

        # Calculate Aggregate Scores
        avg_p1 = (sum(p1_scores) / len(p1_scores)) * 100 if p1_scores else 0.0
        avg_p5 = (sum(p5_scores) / len(p5_scores)) * 100 if p5_scores else 0.0
        total_unique = len(tasks_map)
        pct_pass_all = (pass_all_count / total_unique * 100) if total_unique > 0 else 0.0

        leaderboard.append({
            "id": model,
            "type": get_model_type(model),
            "p1": round(avg_p1, 1),
            "p5": round(avg_p5, 1),
            "pAll": round(pct_pass_all, 1),
            "runs": total_runs_count,
            "tasks": total_unique
        })
        
        # Sort model details by task name, then run number
        model_rows.sort(key=lambda x: (x['task'], x['run']))
        model_details_map[model] = model_rows

    # --- TASK STATS & TASK DETAILS ---
    # We need to invert the grouping to be Task -> Model
    all_tasks = set()
    for m in grouped_raw:
        all_tasks.update(grouped_raw[m].keys())
        
    for t_name in all_tasks:
        # 1. Summary Stats (Across all models)
        all_results_for_task = []
        for m in grouped_raw:
            if t_name in grouped_raw[m]:
                all_results_for_task.extend([str(x.get('result')).lower() for x in grouped_raw[m][t_name]])
        
        n_total = len(all_results_for_task)
        c_total = all_results_for_task.count('success')
        p1_total = pass_at_k_naive(n_total, c_total, 1) * 100
        
        task_stats_summary.append({
            "name": t_name,
            "p1": round(p1_total, 1),
            "count": n_total
        })

        # 2. Detailed Breakdown (Per Model)
        # We want a list of models and their specific performance on this task
        t_breakdown = []
        for model in grouped_raw:
            if t_name in grouped_raw[model]:
                items = grouped_raw[model][t_name]
                n = len(items)
                c = sum(1 for i in items if str(i.get('result')).lower() == 'success')
                p1 = pass_at_k_naive(n, c, 1) * 100
                
                # Create a mini visualization string of results like "S S F S F"
                run_results = []
                for idx, x in enumerate(items):
                     res_code = "S" if str(x.get('result')).lower() == 'success' else "F"
                     run_results.append({"r": idx+1, "val": res_code})

                t_breakdown.append({
                    "model": model,
                    "type": get_model_type(model),
                    "p1": round(p1, 1),
                    "runs": run_results
                })
        
        # Sort breakdown by P1 desc
        t_breakdown.sort(key=lambda x: x['p1'], reverse=True)
        task_details_map[t_name] = t_breakdown

    # Final Sorting
    leaderboard.sort(key=lambda x: x['p5'], reverse=True)
    task_stats_summary.sort(key=lambda x: x['p1']) # Hardest first (lowest score)

    return {
        "leaderboard": leaderboard,
        "tasks": task_stats_summary,
        "details": model_details_map,
        "task_details": task_details_map
    }

# --- 2. HTML GENERATION HELPERS ---

def get_common_head(title, data_json_str=None):
    data_script = ""
    if data_json_str:
        data_script = f"<script>window.BENCHMARK_DATA = {data_json_str};</script>"

    return f"""
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{title}</title>
    {data_script}
    <style>
        body {{ font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif; margin: 0; padding: 0; background-color: #f6f8fa; color: #24292e; display: flex; flex-direction: column; min-height: 100vh; }}
        
        /* Navbar */
        .navbar {{ background-color: #24292e; padding: 1rem 2rem; display: flex; align-items: center; justify-content: space-between; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }}
        .nav-brand {{ color: white; font-weight: bold; font-size: 1.2rem; text-decoration: none; margin-right: 2rem; }}
        .nav-links {{ display: flex; gap: 1rem; }}
        .nav-btn {{ color: rgba(255,255,255,0.7); text-decoration: none; font-weight: 600; padding: 0.5rem 1rem; border-radius: 6px; transition: all 0.2s; font-size: 0.9rem; }}
        .nav-btn:hover {{ color: white; background-color: rgba(255,255,255,0.1); }}
        .nav-btn.active {{ color: white; background-color: #0969da; }}
        
        .container {{ max-width: 1200px; margin: 0 auto; padding: 2rem; width: 100%; box-sizing: border-box; flex: 1; }}
        h1, h2 {{ text-align: center; font-weight: 600; border-bottom: 1px solid #e1e4e8; padding-bottom: 0.5em; margin-top: 1em; }}
        
        /* Tables */
        table {{ width: 100%; border-collapse: collapse; margin: 20px auto; box-shadow: 0 1px 3px rgba(0,0,0,0.1); background-color: #fff; table-layout: fixed; }}
        th, td {{ border: 1px solid #dfe2e5; padding: 12px 15px; text-align: left; vertical-align: middle; }}
        th {{ background-color: #f6f8fa; font-weight: 600; cursor: pointer; user-select: none; position: relative; }}
        th:hover {{ background-color: #eaeef2; }}
        tr:nth-child(even) {{ background-color: #f6f8fa; }}
        
        th::after {{ content: ' \\2195'; position: absolute; right: 8px; opacity: 0.3; font-size: 0.8em; }}
        th.asc::after {{ content: ' \\2191'; opacity: 1; }}
        th.desc::after {{ content: ' \\2193'; opacity: 1; }}

        /* Intro & Controls */
        .intro-container {{ max-width: 800px; margin: 0 auto 2rem auto; text-align: left; color: #57606a; background-color: #fff; padding: 1.5rem; border: 1px solid #e1e4e8; border-radius: 6px; margin-bottom: 2rem; }}
        .intro-row {{ margin-bottom: 0.5rem; line-height: 1.5; }}
        .intro-label {{ font-weight: 600; color: #24292e; margin-right: 5px; }}
        .metric-def {{ margin-bottom: 5px; font-size: 0.95em; }}
        
        .controls-area {{ display: flex; justify-content: center; margin-bottom: 20px; flex-direction: column; align-items: center; gap: 15px; }}
        .control-row {{ display: flex; align-items: center; gap: 15px; flex-wrap: wrap; justify-content: center; }}
        
        /* Buttons & Toggles */
        .toggle-group {{ display: inline-flex; background: #e1e4e8; padding: 4px; border-radius: 6px; }}
        .toggle-btn {{ padding: 8px 16px; border: none; background: transparent; cursor: pointer; font-weight: 600; color: #57606a; border-radius: 4px; transition: all 0.2s; }}
        .toggle-btn:hover {{ color: #24292e; }}
        .toggle-btn.active {{ background: white; color: #0969da; box-shadow: 0 1px 3px rgba(0,0,0,0.1); }}
        
        .checkbox-label {{ font-weight: 600; color: #24292e; user-select: none; cursor: pointer; display: flex; align-items: center; gap: 5px; }}
        
        .disclaimer-box {{ font-size: 0.9em; background-color: #fff8c5; padding: 8px 16px; border-radius: 6px; border: 1px solid #d4a72c; color: #4a3b05; max-width: 600px; display: none; margin-top: 10px; }}
        .disclaimer-box.visible {{ display: block; }}

        /* Score Bars */
        .score-bar-wrapper {{ width: 100%; height: 24px; background-color: #e1e4e8; border-radius: 6px; overflow: hidden; display: flex; position: relative; cursor: help; }}
        .bar-segment {{ height: 100%; }}
        .score-bar-tooltip {{ display: none; position: absolute; left: 50%; top: -30px; transform: translateX(-50%); background: #24292e; color: #fff; padding: 4px 8px; border-radius: 4px; font-size: 0.75em; white-space: nowrap; z-index: 10; }}
        .score-bar-wrapper:hover .score-bar-tooltip {{ display: block; }}
        
        /* Badges & Links */
        .result-badge {{ font-weight: bold; text-transform: capitalize; padding: 4px 8px; border-radius: 4px; color: white; font-size: 0.85em; display: inline-block; min-width: 60px; text-align: center; }}
        .result-badge.success {{ background-color: #2da44e; }}
        .result-badge.fail {{ background-color: #cf222e; }}
        
        .run-dot {{ width: 12px; height: 12px; display: inline-block; border-radius: 2px; margin-right: 2px; }}
        .run-dot.S {{ background-color: #2da44e; }}
        .run-dot.F {{ background-color: #cf222e; }}
        
        .failure-box {{ font-size: 0.9em; margin-bottom: 8px; }}
        .failure-msg {{ font-family: "SFMono-Regular", Consolas, monospace; background-color: #f6f8fa; padding: 8px; border-radius: 6px; border: 1px solid #e1e4e8; white-space: pre-wrap; max-height: 100px; overflow-y: auto; }}
        
        .log-link {{ color: #0969da; text-decoration: none; font-weight: 600; font-size: 0.9em; }}
        .log-link:hover {{ text-decoration: underline; }}
        .model-link {{ text-decoration: none; color: #0969da; font-weight: bold; font-size: 1.05em; }}
        .model-link:hover {{ text-decoration: underline; }}
        .task-link {{ text-decoration: none; color: #24292e; }}
        .task-link:hover {{ color: #0969da; text-decoration: underline; }}
        .back-link {{ display: inline-block; margin-bottom: 1rem; color: #0969da; text-decoration: none; font-weight: 600; }}
        .back-link:hover {{ text-decoration: underline; }}
    </style>
    """

COMMON_SCRIPTS = """
    <script>
        function getHue(percentage) { return (percentage / 100) * 120; }
        
        function createMiniBar(val, hue) {
            return `<div style="height: 6px; width: 100%; background: #eee; border-radius: 3px; margin-top: 5px; overflow: hidden;">
                <div style="height: 100%; width: ${val}%; background-color: hsla(${hue}, 85%, 40%, 1.0);"></div>
            </div>`;
        }

        function sortTable(table, colIndex) {
            const tbody = table.querySelector('tbody');
            const rows = Array.from(tbody.querySelectorAll('tr'));
            const header = table.querySelector(`th[data-idx='${colIndex}']`);
            const isAsc = header.classList.contains('asc');
            const direction = isAsc ? -1 : 1;
            
            rows.sort((a, b) => {
                const aTxt = a.children[colIndex].innerText.trim();
                const bTxt = b.children[colIndex].innerText.trim();
                const aNum = parseFloat(aTxt.replace(/[^0-9.-]+/g,""));
                const bNum = parseFloat(bTxt.replace(/[^0-9.-]+/g,""));
                if (!isNaN(aNum) && !isNaN(bNum) && (aTxt.includes('%') || aTxt.match(/^\\d/))) {
                     return (aNum - bNum) * direction;
                }
                return aTxt.localeCompare(bTxt, undefined, {numeric: true}) * direction;
            });
            tbody.innerHTML = '';
            tbody.append(...rows);
            table.querySelectorAll('th').forEach(th => th.classList.remove('asc', 'desc'));
            header.classList.toggle('asc', !isAsc);
            header.classList.toggle('desc', isAsc);
        }
        
        document.addEventListener('DOMContentLoaded', () => {
            document.querySelectorAll('th[data-idx]').forEach(th => {
                th.addEventListener('click', () => sortTable(th.closest('table'), th.dataset.idx));
            });
            if (window.renderPage) window.renderPage();
        });
    </script>
"""

def get_navbar(active):
    def cls(name): return 'nav-btn active' if active == name else 'nav-btn'
    return f"""
    <nav class="navbar">
        <a href="index.html" class="nav-brand">k8s-ai-bench</a>
        <div class="nav-links">
            <a href="index.html" class="{cls('leaderboard')}">Leaderboard</a>
            <a href="tasks.html" class="{cls('tasks')}">Tasks</a>
            <a href="about.html" class="{cls('about')}">About</a>
            <a href="https://github.com/GoogleCloudPlatform/kubectl-ai" class="nav-btn" target="_blank">GitHub &nearr;</a>
        </div>
    </nav>
    """

# --- 3. PAGE CONTENT GENERATORS ---

def write_index_html(data_json):
    content = f"""
<!DOCTYPE html>
<html lang="en">
<head>
    {get_common_head("k8s-ai-bench Leaderboard", data_json)}
</head>
<body>
    {get_navbar('leaderboard')}
    <div class="container">
        <h1>k8s-ai-bench Leaderboard</h1>
        
        <div class="intro-container">
            <div class="intro-row"><span class="intro-label">Goal:</span> Determine which LLM is best suited for solving Kubernetes tasks.</div>
            <div class="intro-row"><span class="intro-label">Method:</span> Evaluation against 24 specific tasks in the <a href="{REPO_BASE_URL}" target="_blank" style="color: #0969da; text-decoration: none;">k8s-ai-bench</a> repository (totaling 120 execution runs per model).</div>
            <div class="intro-row" style="margin-top: 10px; border-top: 1px dashed #e1e4e8; padding-top: 10px;">
                <div class="intro-label" style="margin-bottom: 5px;">Metrics:</div>
                <div class="metric-def"><strong>Pass@1:</strong> The probability of passing a task on the first try. This represents the average performance of the model.</div>
                <div class="metric-def"><strong>Pass@5:</strong> The probability of passing a task within 5 tries. This represents the potential of the model.</div>
                <div class="metric-def"><strong>Pass All 5:</strong> The percent of tasks that passed on every run.</div>
            </div>
        </div>
        
        <div class="controls-area">
            <div class="control-row">
                <span style="font-weight:600; font-size:0.9em;">Metric:</span>
                <div class="toggle-group">
                    <button class="toggle-btn" onclick="setMetric('p1')" id="btn-p1">Pass@1</button>
                    <button class="toggle-btn active" onclick="setMetric('p5')" id="btn-p5">Pass@5</button>
                    <button class="toggle-btn" onclick="setMetric('pAll')" id="btn-pAll">Pass All 5</button>
                </div>
            </div>
            
            <div class="control-row">
                <span style="font-weight:600; font-size:0.9em;">Filter Models:</span>
                <label class="checkbox-label">
                    <input type="checkbox" id="chk-hosted" checked onchange="renderPage()"> Hosted
                </label>
                <label class="checkbox-label">
                    <input type="checkbox" id="chk-self" checked onchange="renderPage()"> Self-Hosted
                </label>
            </div>
            
            <div class="disclaimer-box" id="pAll-disclaimer">
                <strong>Note:</strong> We are still evaluating whether "Pass All 5" is a robust metric. It represents the percentage of tasks where the model succeeded in every single attempt (Consistency).
            </div>
        </div>
        
        <div style="max-width: 900px; margin: 0 auto;">
            <table id="leaderboard-table">
                <thead>
                    <tr>
                        <th data-idx="0" style="width: 250px;">Model</th>
                        <th data-idx="1">Score</th>
                        <th data-idx="2" style="width: 100px;">Type</th>
                    </tr>
                </thead>
                <tbody></tbody>
            </table>
        </div>
        <p style="text-align:center; color:#6e7781; margin-top: 2rem;">Click on a model name to view detailed logs.</p>
    </div>
    {COMMON_SCRIPTS}
    <script>
        let currentMetric = 'p5'; // Default

        function setMetric(metric) {{
            currentMetric = metric;
            
            // Update Buttons
            document.querySelectorAll('.toggle-btn').forEach(b => b.classList.remove('active'));
            document.getElementById('btn-' + metric).classList.add('active');
            
            // Toggle Disclaimer
            const disclaimer = document.getElementById('pAll-disclaimer');
            if (metric === 'pAll') disclaimer.classList.add('visible');
            else disclaimer.classList.remove('visible');
            
            renderPage();
        }}

        window.renderPage = function() {{
            const tbody = document.querySelector('#leaderboard-table tbody');
            const rawData = window.BENCHMARK_DATA.leaderboard;
            
            const showHosted = document.getElementById('chk-hosted').checked;
            const showSelf = document.getElementById('chk-self').checked;
            
            // Filter
            let data = rawData.filter(row => {{
                if (row.type === 'Hosted' && !showHosted) return false;
                if (row.type === 'Self-Hosted' && !showSelf) return false;
                return true;
            }});
            
            // Sort
            data.sort((a, b) => b[currentMetric] - a[currentMetric]);
            
            tbody.innerHTML = data.map(row => {{
                const val = row[currentMetric];
                const hue = getHue(val);
                
                return `
                <tr>
                    <td><a href="model.html?id=${{encodeURIComponent(row.id)}}" class="model-link">${{row.id}}</a></td>
                    <td>
                        <div class="score-container">
                            <div class="score-text" style="font-weight:bold;">${{val}}%</div>
                            <div class="score-bar-wrapper">
                                <div class="score-bar-tooltip">${{currentMetric}}: ${{val}}%</div>
                                <div class="bar-segment" style="width: ${{val}}%; background-color: hsla(${{hue}}, 85%, 40%, 1.0);"></div>
                            </div>
                        </div>
                    </td>
                    <td><span style="font-size:0.85em; color:#57606a;">${{row.type}}</span></td>
                </tr>`;
            }}).join('');
        }}
    </script>
</body>
</html>
    """
    with open('index.html', 'w', encoding='utf-8') as f: f.write(content)

def write_tasks_html(data_json):
    repo_base = REPO_TASK_URL
    
    content = f"""
<!DOCTYPE html>
<html lang="en">
<head>
    {get_common_head("Task Difficulty Leaderboard", data_json)}
</head>
<body>
    {get_navbar('tasks')}
    <div class="container">
        <h1>Task Difficulty Leaderboard</h1>
        <p style="text-align: center; max-width: 800px; margin: 0 auto; color: #57606a;">
            Click task name to view source on Github. Click 'View Stats' to see task results for all models.
        </p>
        <table id="tasks-table">
            <thead>
                <tr>
                    <th data-idx="0" style="width: 50%;">Task Name</th>
                    <th data-idx="1" style="width: 30%;">Overall Pass@1</th>
                    <th data-idx="2" style="width: 20%;">Details</th>
                </tr>
            </thead>
            <tbody></tbody>
        </table>
    </div>
    {COMMON_SCRIPTS}
    <script>
        window.renderPage = function() {{
            const tbody = document.querySelector('#tasks-table tbody');
            const data = window.BENCHMARK_DATA.tasks;
            const repoUrl = "{repo_base}";
            
            tbody.innerHTML = data.map(t => {{
                const taskUrl = repoUrl + encodeURIComponent(t.name);
                const detailsUrl = 'task_detail.html?id=' + encodeURIComponent(t.name);
                
                return `
                <tr>
                    <td>
                        <a href="${{taskUrl}}" class="task-link" target="_blank" title="View source on GitHub">
                            <strong>${{t.name}}</strong> &nearr;
                        </a>
                    </td>
                    <td>${{t.p1}}% ${{createMiniBar(t.p1, getHue(t.p1))}}</td>
                    <td><a href="${{detailsUrl}}" class="log-link">View Stats &rarr;</a></td>
                </tr>`;
            }}).join('');
        }}
    </script>
</body>
</html>
    """
    with open('tasks.html', 'w', encoding='utf-8') as f: f.write(content)

def write_task_detail_html(data_json):
    content = f"""
<!DOCTYPE html>
<html lang="en">
<head>
    {get_common_head("Task Details", data_json)}
</head>
<body>
    {get_navbar('tasks')}
    <div class="container">
        <a href="tasks.html" class="back-link">&larr; Back to Tasks List</a>
        <h1 id="task-title">Loading...</h1>
        
        <table id="task-detail-table">
            <thead>
                <tr>
                    <th data-idx="0" style="width: 40%;">Model</th>
                    <th data-idx="1" style="width: 20%;">Pass Rate</th>
                    <th data-idx="2" style="width: 40%;">Run Outcomes</th>
                </tr>
            </thead>
            <tbody></tbody>
        </table>
    </div>
    {COMMON_SCRIPTS}
    <script>
        window.renderPage = function() {{
            const params = new URLSearchParams(window.location.search);
            const taskId = params.get('id');
            const data = window.BENCHMARK_DATA.task_details[taskId];
            
            if (!data) {{
                document.getElementById('task-title').innerText = 'Task Not Found';
                return;
            }}
            
            document.getElementById('task-title').innerText = taskId;
            const tbody = document.querySelector('#task-detail-table tbody');
            
            tbody.innerHTML = data.map(row => {{
                // row.runs is array like [{{r:1, val:'S'}}, ...]
                const dots = row.runs.map(r => `<div class="run-dot ${{r.val}}" title="Run ${{r.r}}: ${{r.val}}"></div>`).join('');
                
                return `
                <tr>
                    <td><a href="model.html?id=${{encodeURIComponent(row.model)}}" class="model-link">${{row.model}}</a></td>
                    <td>${{row.p1}}%</td>
                    <td>${{dots}}</td>
                </tr>`;
            }}).join('');
        }}
    </script>
</body>
</html>
    """
    with open('task_detail.html', 'w', encoding='utf-8') as f: f.write(content)

def write_model_html(data_json):
    content = f"""
<!DOCTYPE html>
<html lang="en">
<head>
    {get_common_head("Model Details", data_json)}
</head>
<body>
    {get_navbar('leaderboard')}
    <div class="container">
        <a href="index.html" class="back-link">&larr; Back to Leaderboard</a>
        <h1 id="model-title">Loading...</h1>
        
        <div class="controls-area" style="flex-direction: row; margin-bottom: 1rem;">
            <span style="font-weight:600;">Run Filter:</span>
            <select id="run-select" onchange="renderPage()" style="padding: 5px; font-size:1rem;">
                <option value="all">All Runs</option>
                <option value="1">Run 1</option>
                <option value="2">Run 2</option>
                <option value="3">Run 3</option>
                <option value="4">Run 4</option>
                <option value="5">Run 5</option>
            </select>
        </div>

        <table id="details-table">
            <thead>
                <tr>
                    <th data-idx="0" style="width: 30%;">Task Name</th>
                    <th data-idx="1" style="width: 10%;">Run #</th>
                    <th data-idx="2" style="width: 10%;">Result</th>
                    <th style="width: 50%;">Details</th>
                </tr>
            </thead>
            <tbody></tbody>
        </table>
    </div>
    {COMMON_SCRIPTS}
    <script>
        window.renderPage = function() {{
            const params = new URLSearchParams(window.location.search);
            const modelId = params.get('id');
            const data = window.BENCHMARK_DATA.details[modelId];
            const runFilter = document.getElementById('run-select').value;
            
            if (!data) {{
                document.getElementById('model-title').innerText = 'Model Not Found';
                return;
            }}
            
            document.getElementById('model-title').innerText = 'Details: ' + modelId;
            const tbody = document.querySelector('#details-table tbody');
            
            // Filter Data
            let filteredData = data;
            if (runFilter !== 'all') {{
                const targetRun = parseInt(runFilter);
                filteredData = data.filter(r => r.run === targetRun);
            }}

            tbody.innerHTML = filteredData.map(row => {{
                const badgeClass = row.res === 'success' ? 'success' : 'fail';
                let detailsHTML = '<span style="color:#999">-</span>';
                
                if (row.res === 'fail') {{
                    const msg = row.msg || "No failure message provided.";
                    detailsHTML = `<div class="failure-box"><div class="failure-msg">${{msg}}</div></div>`;
                }}
                
                return `
                <tr>
                    <td>${{row.task}}</td>
                    <td>${{row.run}}</td>
                    <td><span class="result-badge ${{badgeClass}}">${{row.res}}</span></td>
                    <td>${{detailsHTML}}</td>
                </tr>`;
            }}).join('');
        }}
    </script>
</body>
</html>
    """
    with open('model.html', 'w', encoding='utf-8') as f: f.write(content)

def write_about_html():
    content = f"""
<!DOCTYPE html>
<html lang="en">
<head>
    {get_common_head("About")}
</head>
<body>
    {get_navbar('about')}
    <div class="container" style="max-width: 800px;">
        <h1>About Project</h1>
        <div style="background: white; padding: 2rem; border: 1px solid #dfe2e5; border-radius: 6px; line-height: 1.6;">
            <p><strong>k8s-ai-bench</strong> is a benchmarking tool designed to evaluate Large Language Models (LLMs) on their ability to interact with and manage Kubernetes clusters.</p>
            <h3>How it works</h3>
            <p>The benchmark runs a series of 24 predefined tasks against different LLMs. Each task is attempted multiple times to gauge consistency and potential.</p>
        </div>
    </div>
</body>
</html>
    """
    with open('about.html', 'w', encoding='utf-8') as f: f.write(content)

# --- 4. MAIN EXECUTION ---

def main():
    if not os.path.exists(INPUT_FILE):
        print(f"Error: {INPUT_FILE} not found.")
        return

    print("Loading JSON Lines...")
    raw_data = []
    with open(INPUT_FILE, 'r') as f:
        for line in f:
            if line.strip():
                raw_data.append(json.loads(line))

    print(f"Loaded {len(raw_data)} records.")
    print("Processing Statistics...")
    
    processed_data = process_data(raw_data)
    data_json_str = json.dumps(processed_data)

    print("Writing HTML Files...")
    write_index_html(data_json_str)
    write_tasks_html(data_json_str)
    write_task_detail_html(data_json_str)
    write_model_html(data_json_str)
    write_about_html()

    print("✅ Done! Generated index.html, tasks.html, task_detail.html, model.html, about.html")

if __name__ == "__main__":
    main()