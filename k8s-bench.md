# K8s-bench Evaluation Results

## Google

### Model Performance Summary

| Model | Success | Fail |
|-------|---------|------|
| gemini-2.5-flash-preview-04-17 | 10 | 0 |
| gemini-2.5-pro-preview-03-25 | 10 | 0 |
| gemma-3-27b-it | 8 | 2 |
| **Total** | 28 | 2 |

### Overall Summary

- Total Runs: 30
- Overall Success: 28 (93%)
- Overall Fail: 2 (6%)

### Model: gemini-2.5-flash-preview-04-17

| Task | Provider | Result |
|------|----------|--------|
| create-network-policy | gemini | ✅ success |
| create-pod | gemini | ✅ success |
| create-pod-mount-configmaps | gemini | ✅ success |
| create-pod-resources-limits | gemini | ✅ success |
| fix-crashloop | gemini | ✅ success |
| fix-image-pull | gemini | ✅ success |
| fix-service-routing | gemini | ✅ success |
| list-images-for-pods | gemini | ✅ success |
| scale-deployment | gemini | ✅ success |
| scale-down-deployment | gemini | ✅ success |

**gemini-2.5-flash-preview-04-17 Summary**

- Total: 10
- Success: 10 (100%)
- Fail: 0 (0%)

### Model: gemini-2.5-pro-preview-03-25

| Task | Provider | Result |
|------|----------|--------|
| create-network-policy | gemini | ✅ success |
| create-pod | gemini | ✅ success |
| create-pod-mount-configmaps | gemini | ✅ success |
| create-pod-resources-limits | gemini | ✅ success |
| fix-crashloop | gemini | ✅ success |
| fix-image-pull | gemini | ✅ success |
| fix-service-routing | gemini | ✅ success |
| list-images-for-pods | gemini | ✅ success |
| scale-deployment | gemini | ✅ success |
| scale-down-deployment | gemini | ✅ success |

**gemini-2.5-pro-preview-03-25 Summary**

- Total: 10
- Success: 10 (100%)
- Fail: 0 (0%)

### Model: gemma-3-27b-it

| Task | Provider | Result |
|------|----------|--------|
| create-network-policy | gemini | ❌  |
| create-pod | gemini | ✅ success |
| create-pod-mount-configmaps | gemini | ✅ success |
| create-pod-resources-limits | gemini | ✅ success |
| fix-crashloop | gemini | ✅ success |
| fix-image-pull | gemini | ✅ success |
| fix-service-routing | gemini | ❌  |
| list-images-for-pods | gemini | ✅ success |
| scale-deployment | gemini | ✅ success |
| scale-down-deployment | gemini | ✅ success |

**gemma-3-27b-it Summary**

- Total: 10
- Success: 8 (80%)
- Fail: 2 (20%)

---

_Report generated on April 24, 2025 at 11:42 AM_

## OpenAI

### Model Performance Summary

| Model | Success | Fail |
|-------|---------|------|
| gpt-4.1 | 8 | 2 |
| gpt-4o | 0 | 10 |
| o4-mini | 0 | 10 |
| **Total** | 8 | 22 |

### Overall Summary

- Total Runs: 30
- Overall Success: 8 (26%)
- Overall Fail: 22 (73%)

### Model: gpt-4.1

| Task | Provider | Result |
|------|----------|--------|
| create-network-policy | openai | ❌  |
| create-pod | openai | ✅ success |
| create-pod-mount-configmaps | openai | ✅ success |
| create-pod-resources-limits | openai | ❌  |
| fix-crashloop | openai | ✅ success |
| fix-image-pull | openai | ✅ success |
| fix-service-routing | openai | ✅ success |
| list-images-for-pods | openai | ✅ success |
| scale-deployment | openai | ✅ success |
| scale-down-deployment | openai | ✅ success |

**gpt-4.1 Summary**

- Total: 10
- Success: 8 (80%)
- Fail: 2 (20%)

### Model: gpt-4o

| Task | Provider | Result |
|------|----------|--------|
| create-network-policy | openai | ❌  |
| create-pod | openai | ❌  |
| create-pod-mount-configmaps | openai | ❌  |
| create-pod-resources-limits | openai | ❌  |
| fix-crashloop | openai | ❌  |
| fix-image-pull | openai | ❌  |
| fix-service-routing | openai | ❌  |
| list-images-for-pods | openai | ❌  |
| scale-deployment | openai | ❌  |
| scale-down-deployment | openai | ❌  |

**gpt-4o Summary**

- Total: 10
- Success: 0 (0%)
- Fail: 10 (100%)

### Model: o4-mini

| Task | Provider | Result |
|------|----------|--------|
| create-network-policy | openai | ❌  |
| create-pod | openai | ❌  |
| create-pod-mount-configmaps | openai | ❌  |
| create-pod-resources-limits | openai | ❌  |
| fix-crashloop | openai | ❌  |
| fix-image-pull | openai | ❌  |
| fix-service-routing | openai | ❌  |
| list-images-for-pods | openai | ❌  |
| scale-deployment | openai | ❌  |
| scale-down-deployment | openai | ❌  |

**o4-mini Summary**

- Total: 10
- Success: 0 (0%)
- Fail: 10 (100%)

---

_Report generated on May 7, 2025 at 10:04 PM_