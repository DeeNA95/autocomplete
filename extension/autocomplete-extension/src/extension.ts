// The module 'vscode' contains the VS Code extensibility API
// Import the module and reference it with the alias vscode in your code below
import * as vscode from "vscode";
import { spawn, ChildProcess } from "child_process";
import * as path from "path";
import { ApiClient } from "./apiClient";
import find from "find-process";

let backendProcess: ChildProcess;
let apiClient: ApiClient;
let statusBar: vscode.StatusBarItem;
let outputChannel: vscode.OutputChannel;
let debounceTimer: NodeJS.Timeout | undefined;
let ghostTextProvider: vscode.Disposable | undefined;

function isPathIgnored(filePath: string): boolean {
  const ignoredDirs = [
    "node_modules",
    "vendor",
    "dist",
    "build",
    "__pycache__",
    "venv",
  ];
  const ignoredFiles = [
    "package-lock.json",
    "yarn.lock",
    "pnpm-lock.yaml",
    "go.sum",
  ];
  const ignoredExtensions = [".lock", ".svg", ".png"];

  const pathParts = filePath.split(path.sep);
  const basename = path.basename(filePath);

  // Check for ignored directories or directories starting with '.'
  for (const part of pathParts) {
    if (
      ignoredDirs.includes(part) ||
      (part.startsWith(".") && part.length > 1)
    ) {
      return true;
    }
  }

  // Check for ignored files or files starting with '.'
  if (
    ignoredFiles.includes(basename) ||
    (basename.startsWith(".") && basename.length > 1)
  ) {
    return true;
  }

  // Check for ignored extensions
  for (const ext of ignoredExtensions) {
    if (basename.endsWith(ext)) {
      return true;
    }
  }

  return false;
}
// import { spawn } from "child_process";

function startBackendProcess(
  context: vscode.ExtensionContext,
  openaiApiKey: string,
  serverPath: string,
  embeddingConfig: any,
): Promise<void> {
  return new Promise((resolve, reject) => {
    outputChannel.appendLine("Starting backend process...");

    if (!openaiApiKey) {
      reject(
        new Error(
          "FATAL: OpenAI API Key must be provided from VS Code secrets.",
        ),
      );
      return;
    }

    // Read exclude settings from VSCode configuration
    const configuration = vscode.workspace.getConfiguration("autocomplete");
    const excludedFiles: string[] = configuration.get("excludedFiles", []);
    const excludedExtensions: string[] = configuration.get(
      "excludedExtensions",
      [],
    );

    // Compose environment variables from config and exclude lists
    const env = {
      ...process.env,
      OPENAI_API_KEY_INJECTED: openaiApiKey,
      EMBEDDING_PROVIDER: embeddingConfig.provider,
      OPENAI_EMBEDDING_MODEL: embeddingConfig.openai.model,
      LOCAL_EMBEDDING_URL: embeddingConfig.local.serverUrl,
      LOCAL_EMBEDDING_SERVER_TYPE: embeddingConfig.local.serverType,
      LOCAL_EMBEDDING_MODEL: embeddingConfig.local.modelName,
      LOCAL_EMBEDDING_TIMEOUT: embeddingConfig.local.timeout.toString(),
      HUGGINGFACE_MODEL_ID: embeddingConfig.huggingface.modelId,
      HUGGINGFACE_CACHE_DIR: embeddingConfig.huggingface.cacheDir,
      HUGGINGFACE_USE_GPU: embeddingConfig.huggingface.useGpu.toString(),
      HUGGINGFACE_MAX_LENGTH: embeddingConfig.huggingface.maxLength.toString(),
      HUGGINGFACE_BATCH_SIZE: embeddingConfig.huggingface.batchSize.toString(),
      EXCLUDED_FILES: excludedFiles.join(","),
      EXCLUDED_EXTENSIONS: excludedExtensions.join(","),
    };

    outputChannel.appendLine(
      `Using embedding provider: ${embeddingConfig.provider}`,
    );
    if (embeddingConfig.provider === "local") {
      outputChannel.appendLine(
        `Local server URL: ${embeddingConfig.local.serverUrl}`,
      );
      outputChannel.appendLine(
        `Local server type: ${embeddingConfig.local.serverType}`,
      );
    } else if (embeddingConfig.provider === "huggingface") {
      outputChannel.appendLine(
        `HuggingFace model: ${embeddingConfig.huggingface.modelId}`,
      );
      outputChannel.appendLine(
        `Cache directory: ${embeddingConfig.huggingface.cacheDir}`,
      );
      outputChannel.appendLine(
        `GPU acceleration: ${embeddingConfig.huggingface.useGpu}`,
      );
    }

    backendProcess = spawn(serverPath, [], {
      cwd: context.extensionPath,
      env: env,
    });

    backendProcess.on("error", (err: Error) => {
      outputChannel.appendLine(`Failed to start backend process: ${err}`);
      statusBar.text = "$(error) Backend failed";
      reject(err);
    });

    backendProcess.stdout?.on("data", (data: Buffer) => {
      const message = data.toString();
      outputChannel.appendLine(`Backend: ${message}`);
      if (message.includes("Listening and serving HTTP on")) {
        outputChannel.appendLine("Backend is ready.");
        resolve();
      }
    });

    backendProcess.stderr?.on("data", (data: Buffer) => {
      outputChannel.appendLine(`Backend ERROR: ${data}`);
    });

    backendProcess.on("close", (code: number) => {
      if (code !== 0) {
        const errorMessage = `Backend process exited with code ${code}`;
        outputChannel.appendLine(errorMessage);
        reject(new Error(errorMessage));
      }
    });
  });
}

export async function activate(context: vscode.ExtensionContext) {
  // Create an output channel for logging.
  outputChannel = vscode.window.createOutputChannel("AI Autocomplete");
  context.subscriptions.push(outputChannel);
  outputChannel.appendLine(
    'Congratulations, your extension "autocomplete-extension" is now active!',
  );

  // Create a decoration for the loading animation.
  const loadingDecorationType = vscode.window.createTextEditorDecorationType({
    after: {
      contentText: "...",
      color: new vscode.ThemeColor("editorCodeLens.foreground"), // A subtle color
      margin: "0 0 0 1em",
    },
    rangeBehavior: vscode.DecorationRangeBehavior.ClosedClosed,
  });
  context.subscriptions.push(loadingDecorationType);

  // Register the command to show logs.
  const showLogsCommand = vscode.commands.registerCommand(
    "ai-autocomplete.showLogs",
    () => {
      outputChannel.show();
    },
  );
  context.subscriptions.push(showLogsCommand);

  const config = vscode.workspace.getConfiguration("autocomplete");
  const port = config.get<number>("port") ?? 2539;
  const serverPath = path.join(context.extensionPath, "server");

  // Read embedding configuration
  const embeddingConfig = {
    provider: config.get<string>("embeddingProvider") ?? "openai",
    openai: {
      model: config.get<string>("openai.model") ?? "text-embedding-3-small",
    },
    local: {
      serverUrl:
        config.get<string>("local.serverUrl") ?? "http://localhost:8080",
      serverType: config.get<string>("local.serverType") ?? "tei",
      modelName: config.get<string>("local.modelName") ?? "",
      timeout: config.get<number>("local.timeout") ?? 30,
    },
    huggingface: {
      modelId:
        config.get<string>("huggingface.modelId") ?? "BAAI/bge-small-en-v1.5",
      cacheDir: config.get<string>("huggingface.cacheDir") ?? "./models",
      useGpu: config.get<boolean>("huggingface.useGpu") ?? false,
      maxLength: config.get<number>("huggingface.maxLength") ?? 512,
      batchSize: config.get<number>("huggingface.batchSize") ?? 1,
    },
  };

  apiClient = new ApiClient(port);

  // Create and show the status bar item.
  statusBar = vscode.window.createStatusBarItem(
    vscode.StatusBarAlignment.Left,
    100,
  );
  context.subscriptions.push(statusBar);
  statusBar.text = "$(sync~spin) Activating...";
  statusBar.command = "ai-autocomplete.showLogs";
  statusBar.show();

  // Command to set the OpenAI API key.
  const setApiKeyCommand = vscode.commands.registerCommand(
    "autocomplete.setOpenAIApiKey",
    async () => {
      const apiKey = await vscode.window.showInputBox({
        prompt: "Enter your OpenAI API Key",
        password: true,
        ignoreFocusOut: true,
      });
      if (apiKey) {
        await context.secrets.store("openai.apiKey", apiKey);
        vscode.window.showInformationMessage(
          "OpenAI API Key stored successfully.",
        );
        outputChannel.appendLine("OpenAI API Key stored successfully.");
      }
    },
  );
  context.subscriptions.push(setApiKeyCommand);

  const indexWorkspaceCommand = vscode.commands.registerCommand(
    "autocomplete.indexWorkspace",
    async () => {
      if (!vscode.workspace.workspaceFolders) {
        vscode.window.showWarningMessage("No workspace open to index.");
        return;
      }
      const workspacePath = vscode.workspace.workspaceFolders[0].uri.fsPath;
      statusBar.text = "$(sync~spin) Indexing...";
      outputChannel.appendLine(
        `Manual indexing requested for workspace: ${workspacePath}`,
      );
      try {
        await apiClient.startIndexing(workspacePath);
        statusBar.text = "$(check) Ready";
        outputChannel.appendLine("Manual indexing complete.");
        vscode.window.showInformationMessage("Workspace indexing complete.");
      } catch (error) {
        statusBar.text = "$(error) Indexing failed";
        outputChannel.appendLine(`Manual indexing failed: ${error}`);
        vscode.window.showErrorMessage("Workspace indexing failed.");
      }
    },
  );
  context.subscriptions.push(indexWorkspaceCommand);

  const triggerCompletionCommand = vscode.commands.registerCommand(
    "autocomplete.triggerCompletion",
    async () => {
      const editor = vscode.window.activeTextEditor;
      if (!editor) {
        return;
      }
      const position = editor.selection.active;
      const document = editor.document;
      const textBeforeCursor = document.getText(
        new vscode.Range(new vscode.Position(0, 0), position),
      );

      statusBar.text = "$(sync~spin) Suggesting...";
      outputChannel.appendLine(
        `[INFO] Manually requesting completion for ${document.fileName}`,
      );
      outputChannel.appendLine(
        `[DEBUG] Text before cursor:\n---\n${textBeforeCursor}\n---`,
      );

      try {
        const completion = await apiClient.getCompletionSimple(
          document.fileName,
          textBeforeCursor,
        );
        if (completion) {
          outputChannel.appendLine(
            `[INFO] Received manual completion: "${completion}"`,
          );
          vscode.commands.executeCommand("editor.action.inlineSuggest.trigger");
        } else {
          outputChannel.appendLine(`[INFO] Received empty manual completion.`);
          vscode.window.showInformationMessage("No completion found.");
        }
      } catch (error: any) {
        outputChannel.appendLine(
          `[ERROR] Failed to get manual completion: ${error.message}`,
        );
        statusBar.text = "$(error) Completion Failed";
      } finally {
        statusBar.text = "$(check) Ready";
      }
    },
  );
  context.subscriptions.push(triggerCompletionCommand);

  // Register ghost text completion provider
  ghostTextProvider = vscode.languages.registerInlineCompletionItemProvider(
    { scheme: "file" },
    {
      provideInlineCompletionItems: async (
        document,
        position,
        context,
        token,
      ) => {
        outputChannel.appendLine(
          `[DEBUG] Completion triggered - Kind: ${context.triggerKind}, Document: ${document.fileName}, Position: ${position.line}:${position.character}`,
        );

        // Skip if user is in the middle of accepting a completion
        if (
          context.triggerKind ===
            vscode.InlineCompletionTriggerKind.Automatic &&
          context.selectedCompletionInfo
        ) {
          outputChannel.appendLine(
            `[DEBUG] Skipping - user accepting existing completion`,
          );
          return [];
        }

        // Debounce typing to avoid too many requests
        if (debounceTimer) {
          clearTimeout(debounceTimer);
          outputChannel.appendLine(`[DEBUG] Clearing previous debounce timer`);
        }

        return new Promise((resolve) => {
          debounceTimer = setTimeout(async () => {
            if (token.isCancellationRequested) {
              outputChannel.appendLine(
                `[DEBUG] Request cancelled before execution`,
              );
              resolve([]);
              return;
            }

            try {
              const textBeforeCursor = document.getText(
                new vscode.Range(new vscode.Position(0, 0), position),
              );

              outputChannel.appendLine(
                `[DEBUG] Text before cursor (${textBeforeCursor.trim().length} chars): "${textBeforeCursor.substring(textBeforeCursor.length - 50)}"`,
              );

              // Skip very short inputs or whitespace-only
              if (textBeforeCursor.trim().length < 5) {
                outputChannel.appendLine(
                  `[DEBUG] Skipping - text too short (${textBeforeCursor.trim().length} chars)`,
                );
                resolve([]);
                return;
              }

              outputChannel.appendLine(
                `[INFO] ✅ Requesting completion for ${document.fileName} at ${position.line}:${position.character}`,
              );

              const completion = await apiClient.getCompletionSimple(
                document.fileName,
                textBeforeCursor,
              );

              if (completion && completion.trim()) {
                outputChannel.appendLine(
                  `[INFO] ✅ Ghost text completion received: "${completion.substring(0, 50)}..."`,
                );
                outputChannel.appendLine(
                  `[INFO] 🎯 Showing ghost text to user now!`,
                );
                resolve([new vscode.InlineCompletionItem(completion.trim())]);
              } else {
                outputChannel.appendLine(`[INFO] ❌ Empty completion received`);
                resolve([]);
              }
            } catch (error: any) {
              outputChannel.appendLine(
                `[ERROR] ❌ Ghost text completion failed: ${error.message}`,
              );
              resolve([]);
            }
          }, 300); // 300ms debounce for faster response
        });
      },
    },
  );

  context.subscriptions.push(ghostTextProvider);
  outputChannel.appendLine(
    "[SUCCESS] Ghost text completion provider registered successfully.",
  );

  // Get the API key from secret storage.
  const openaiApiKey = await context.secrets.get("openai.apiKey");

  if (!openaiApiKey) {
    vscode.window.showErrorMessage(
      'FATAL: OpenAI API Key not set. Please run the "Set OpenAI API Key" command.',
    );
    outputChannel.appendLine("FATAL: OpenAI API Key not set.");
    statusBar.text = "$(error) API Key not set";
    return;
  }

  try {
    // Check if the port is already in use and kill the process if it is.
    try {
      const processes = await find("port", port);
      if (processes.length > 0) {
        outputChannel.appendLine(
          `Port ${port} is already in use by process ${processes[0].name} (pid: ${processes[0].pid}). Terminating it.`,
        );
        process.kill(processes[0].pid);
        outputChannel.appendLine(`Process ${processes[0].pid} terminated.`);
      }
    } catch (err) {
      outputChannel.appendLine(`Error checking port ${port}: ${err}`);
    }

    await startBackendProcess(
      context,
      openaiApiKey,
      serverPath,
      embeddingConfig,
    );

    // Trigger indexing when a workspace is opened.
    if (vscode.workspace.workspaceFolders) {
      const workspacePath = vscode.workspace.workspaceFolders[0].uri.fsPath;
      statusBar.text = "$(sync~spin) Indexing...";
      outputChannel.appendLine(`Indexing workspace: ${workspacePath}`);
      await apiClient.startIndexing(workspacePath);
      outputChannel.appendLine(`Started indexing: ${workspacePath}`);
      statusBar.text = "$(check) Ready";
    } else {
      statusBar.text = "$(warning) No workspace open";
      outputChannel.appendLine("No workspace open, skipping indexing.");
    }

    // File watcher for automatic re-indexing.
    const config = vscode.workspace.getConfiguration("files");
    const exclude = config.get<{ [key: string]: boolean }>("exclude", {});
    if (!exclude["**/.git"]) {
      exclude["**/.git"] = true;
      await config.update(
        "exclude",
        exclude,
        vscode.ConfigurationTarget.Global,
      );
    }
    const watcher = vscode.workspace.createFileSystemWatcher(
      "**/*",
      false,
      false,
      true,
    );

    watcher.onDidCreate(async (uri: vscode.Uri) => {
      if (isPathIgnored(uri.fsPath)) {
        outputChannel.appendLine(`Ignoring created file: ${uri.fsPath}`);
        return;
      }
      outputChannel.appendLine(`File created: ${uri.fsPath}, indexing...`);
      statusBar.text = `$(sync~spin) Indexing ${path.basename(uri.fsPath)}...`;
      try {
        await apiClient.indexFile(uri.fsPath);
        statusBar.text = "$(check) Ready";
        outputChannel.appendLine(`Indexing complete for ${uri.fsPath}`);
      } catch (error) {
        statusBar.text = "$(error) Indexing failed";
        outputChannel.appendLine(`Indexing failed for ${uri.fsPath}: ${error}`);
      }
    });

    watcher.onDidChange(async (uri: vscode.Uri) => {
      if (isPathIgnored(uri.fsPath)) {
        outputChannel.appendLine(`Ignoring changed file: ${uri.fsPath}`);
        return;
      }
      outputChannel.appendLine(`File changed: ${uri.fsPath}, indexing...`);
      statusBar.text = `$(sync~spin) Indexing ${path.basename(uri.fsPath)}...`;
      try {
        await apiClient.indexFile(uri.fsPath);
        statusBar.text = "$(check) Ready";
        outputChannel.appendLine(`Indexing complete for ${uri.fsPath}`);
      } catch (error) {
        statusBar.text = "$(error) Indexing failed";
        outputChannel.appendLine(`Indexing failed for ${uri.fsPath}: ${error}`);
      }
    });

    watcher.onDidDelete(async (uri: vscode.Uri) => {
      if (isPathIgnored(uri.fsPath)) {
        outputChannel.appendLine(`Ignoring deleted file: ${uri.fsPath}`);
        return;
      }
      outputChannel.appendLine(
        `File deleted: ${uri.fsPath}, removing from index...`,
      );
      statusBar.text = `$(sync~spin) Deleting ${path.basename(uri.fsPath)}...`;
      try {
        await apiClient.deleteFile(uri.fsPath);
        statusBar.text = "$(check) Ready";
        outputChannel.appendLine(`Deletion complete for ${uri.fsPath}`);
      } catch (error) {
        statusBar.text = "$(error) Deletion failed";
        outputChannel.appendLine(`Deletion failed for ${uri.fsPath}: ${error}`);
      }
    });
    context.subscriptions.push(watcher);
  } catch (err) {
    outputChannel.appendLine(`Failed to start backend or index: ${err}`);
    statusBar.text = "$(error) Initialization failed";
    return; // Stop activation if backend fails to start
  }
}

export function deactivate() {
  if (backendProcess) {
    backendProcess.kill();
  }
  if (statusBar) {
    statusBar.dispose();
  }
}
