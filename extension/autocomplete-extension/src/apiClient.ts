import axios from "axios";

export class ApiClient {
  private client: axios.AxiosInstance;
  private baseUrl: string;

  constructor(port: number) {
    this.baseUrl = `http://localhost:${port}`;
    this.client = axios.create({
      baseURL: this.baseUrl,
    });
  }

  async startIndexing(path: string): Promise<void> {
    await this.client.post("/index", { path });
  }

  async indexFile(path: string): Promise<void> {
    await this.client.post("/index-file", { path });
  }

  async deleteFile(path: string): Promise<void> {
    await this.client.delete("/index-file", { data: { path } });
  }

  async getCompletionSimple(
    filePath: string,
    content: string,
  ): Promise<string> {
    const response = await this.client.get("/complete", {
      params: {
        file_path: filePath,
        content: content,
      },
    });
    return response.data.completion;
  }
}
