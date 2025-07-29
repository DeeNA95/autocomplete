# AI Autocomplete Extension for VS Code

This VS Code extension provides advanced AI-powered code completions to accelerate your development workflow. It supports standard code files as well as Jupyter Notebooks, delivering context-aware suggestions powered by a local indexing backend and AI models.

## Features

- Local backend server indexes your project files into a vector database for fast similarity search
- Contextual AI-generated code completions using OpenAI or configurable embedding providers
- Support for excluding specific file names and extensions from indexing
- Easy API key management via VS Code secret storage
- Seamless integration with your existing workflow through VS Code commands and UI elements

## Getting Started

### 1. Install the Extension

- Package the extension using your preferred method (e.g., `vsce package`) and install the resulting `.vsix` file in VS Code.
- Alternatively, if published, install via the VS Code Marketplace.

### 2. Set Your OpenAI API Key

To enable AI completions, you need to provide your OpenAI API key securely:

1. Open the **Command Palette**:
   - On macOS: `Cmd + Shift + P`
   - On Windows/Linux: `Ctrl + Shift + P`
2. Search for `Set OpenAI API Key` and select it.
3. When prompted, enter your OpenAI API key. The key will be securely stored in VS Code's secret storage.
4. You can update or reset the key anytime by running the same command.

> **Note:** The extension requires this key to connect to OpenAI for generating code completions.

### 3. Open a Project Folder

- Open your project folder/workspace in VS Code.
- The extension will automatically start indexing your workspace files in the background, excluding files and extensions configured in settings.
- You can manually trigger indexing via the `Autocomplete: Index Workspace` command from the Command Palette.

## Configuration Settings

You can customize the extension behavior through the following settings (access via VS Code Settings or `settings.json`):

- `autocomplete.excludedFiles`: Array of file names to exclude from indexing (e.g., `["README.md", "LICENSE"]`)
- `autocomplete.excludedExtensions`: Array of file extensions (without dot) to exclude from indexing (e.g., `["log", "bin"]`)
- `autocomplete.port`: Port used by the local backend server (default: 2539)
- `autocomplete.embeddingProvider`: Choose embedding provider (`openai`, `local`, `huggingface`)
- Additional settings for fine-tuning OpenAI and other embedding providers are available.

## Usage

- After indexing completes, start typing in supported file types.
- Trigger completions using standard VS Code shortcuts or the command `Autocomplete: Trigger Completion`.
- View backend logs using `AI Autocomplete: Show Logs` command for diagnostics.

## Troubleshooting

- **API Key missing or invalid:** Ensure your OpenAI API key is set via the provided command. Check the output channel for errors.
- **Indexing issues:** Confirm that excluded files/extensions are configured correctly if some files aren’t indexed.
- **Performance:** Large workspaces may take some time to index initially; indexing runs asynchronously.

## Contributing

Contributions and feedback are welcome! Please submit issues or pull requests on the project’s GitHub repository.

## License

This extension (and its backend) is released under the MIT License.

---

Happy coding with AI-powered autocomplete!