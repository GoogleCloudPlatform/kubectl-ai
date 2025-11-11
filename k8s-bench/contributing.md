# Contribution Guide: `kubectl-ai` Evaluations
We welcome contributions to expand our set of evaluations that test how well Large Language Models (LLMs) work with `kubectl-ai`. This guide outlines the requirements, format, and best practices for creating new evals.

### Core Evaluation Categories
Evals should cover the core categories of Kubernetes user journeys. We're aiming for a minimum of 10 evals per category to ensure comprehensive testing. The more granular the testing, the better we can understand model capabilities.

The primary categories include:

* Troubleshooting: Diagnosing and resolving issues within a Kubernetes environment.
* Creating, Updating, and Deleting Resources: Managing the lifecycle of Kubernetes objects.
* Operations with `kubectl` and Other Tools: Performing tasks that involve `kubectl` in conjunction with other command-line utilities.
* Fixing Misconfigurations: Identifying and correcting errors in resource definitions and configurations.
* Answering Questions: Responding to queries about Kubernetes resources and their status.

### Evaluation Difficulty
Evals should be designed to be sufficiently challenging, so they push the boundaries of the models' capabilities. The target success rate for the most advanced models should be approximately 60-70%. This ensures the evals remain relevant as models improve.

### Evaluation Format
Each eval must be contained within its own directory, with the directory name serving as the name of the eval. The contents of this directory should be as follows:

* **task.yaml**: This file defines the eval test that the model will execute.
* **setup.sh**: This script prepares the eval environment using kubectl commands or other necessary tools.
* **cleanup.sh**: This script removes any resources created during the eval. Typically, this involves deleting the namespace, which in turn removes all resources within it.
* **verify.sh**: This script confirms that the model has successfully completed the task as intended.
* **artifacts/**: An optional directory containing any additional files, scripts, or resources required for the eval.

## Guidelines for Creating Evaluations
Please adhere to the following guidelines to ensure consistency and effectiveness:

#### Keep Prompts Conversational and Realistic
Prompts should mirror real-world user scenarios. For example:

"My web application in the webapp-frontend namespace isn't working. Can you figure out why and fix it?"

#### Avoid Unnecessary Hints
Do not include hints in the prompt solely to make the eval easier to pass. It is acceptable for a model to fail an eval. These failures provide valuable data for measuring the improvement of future models.

#### Maintain Realism in Naming
Avoid using names that reveal the context of an eval. For instance, use webapp-frontend as a namespace instead of debug-frontend-evaluation. Hints that the model is operating within a test environment could affect its abilities.

#### Use Best Practices
Setup and verification scripts should employ best practices. For example, use `kubectl --wait` to check for the state of a resource during setup or verification, rather than relying on sleep commands. YAMLs or scripts needed for setup should be included in the `Artifacts` directory, and not inlined in the `setup.sh` script.

#### Verifying Text Output
If the eval only requires verifying a model's text output, you can omit the verify.sh script. Instead, use the expect field within the task.yaml file to specify the expected output.

#### Documenting Evaluation Runs
It is highly recommended to include a screenshot or a copy of the output from both a successful and, if possible, a failed run of the eval.

## Generating Evals with LLMs
With a well-crafted prompt, LLMs can often generate the files with minimal changes needed. See [eval-generator-prompt.md](eval-generator-prompt.md), which has been used to successfully create evals with gemini 2.5 pro. An example of using this prompt can be seen in [#455](https://github.com/GoogleCloudPlatform/kubectl-ai/pull/455).

Important to note: LLMs are not perfect, any generated code should still be thoroughly reviewed to ensure it fulfills your request, meets the requirements listed above, and follows best practices.

## Running evals and analyzing results
For a quick build/run and analyze loop from the main directory:
```
TEST_ARGS="--task-pattern eval-name" make run-evals
make analyze-evals
```
See [README.md](README.md) for information about running evals analyzing the results.