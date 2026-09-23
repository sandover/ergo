import * as path from "node:path";
import * as vscode from "vscode";

type Listener = () => void;

export class ErgoLiveness implements vscode.Disposable {
  private readonly projects = new Map<string, ProjectLiveness>();

  subscribe(folder: string, listener: Listener): vscode.Disposable {
    const key = path.resolve(folder);
    let project = this.projects.get(key);
    if (!project) {
      project = new ProjectLiveness(key, () => this.projects.delete(key));
      this.projects.set(key, project);
    }
    return project.subscribe(listener);
  }

  dispose(): void {
    for (const project of this.projects.values()) {
      project.dispose();
    }
    this.projects.clear();
  }
}

class ProjectLiveness implements vscode.Disposable {
  private readonly emitter = new vscode.EventEmitter<void>();
  private readonly watchers: vscode.FileSystemWatcher[];
  private listeners = 0;
  private timer: NodeJS.Timeout | undefined;
  private disposed = false;

  constructor(
    folder: string,
    private readonly onEmpty: () => void,
  ) {
    const dataFolder = path.join(folder, ".ergo");
    this.watchers = ["backlog.jsonl", "journal.jsonl"].map((name) => {
      const watcher = vscode.workspace.createFileSystemWatcher(
        new vscode.RelativePattern(dataFolder, name),
      );
      watcher.onDidChange(() => this.schedule());
      watcher.onDidCreate(() => this.schedule());
      watcher.onDidDelete(() => this.schedule());
      return watcher;
    });
  }

  subscribe(listener: Listener): vscode.Disposable {
    this.listeners++;
    const subscription = this.emitter.event(listener);
    let active = true;
    return new vscode.Disposable(() => {
      if (!active) {
        return;
      }
      active = false;
      subscription.dispose();
      this.listeners--;
      if (this.listeners === 0) {
        this.onEmpty();
        this.dispose();
      }
    });
  }

  dispose(): void {
    if (this.disposed) {
      return;
    }
    this.disposed = true;
    if (this.timer) {
      clearTimeout(this.timer);
    }
    for (const watcher of this.watchers) {
      watcher.dispose();
    }
    this.emitter.dispose();
  }

  private schedule(): void {
    if (this.disposed) {
      return;
    }
    if (this.timer) {
      clearTimeout(this.timer);
    }
    this.timer = setTimeout(() => {
      this.timer = undefined;
      if (!this.disposed) {
        this.emitter.fire();
      }
    }, 100);
  }
}
