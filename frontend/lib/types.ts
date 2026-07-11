export type Status = "Queued" | "Running" | "Success" | "Failed";
export type Level = "platinum" | "gold" | "bronze" | "silver" | "hard" | "medium" | "easy" | "";

export interface User {
  id: string;
  username: string;
  nickname: string;
  signature: string;
  avatar_url: string;
  tags: string;
  role: "user" | "admin" | "superadmin";
  // Admin-only fields (used by admin components; absent for regular users)
  banned_until?: string | null;
  ban_reason?: string;
  disable_rank?: boolean;
}

export interface Announcement {
  id: string;
  title: string;
  created_at: string;
  updated_at: string;
  description: string;
}

export interface RegistrationConfig {
  mode: "auto" | "tag_auto" | "tag_review" | "review";
  allowed_tags?: string[];
}

export interface ContestRegistration {
  id: string;
  contest_id: string;
  user_id: string;
  user: { nickname: string; username: string; tags: string };
  status: "approved" | "pending" | "rejected";
  created_at: string;
  reviewed_at?: string;
}

export interface Contest {
  id: string;
  name: string;
  starttime: string;
  endtime: string;
  submit_start_time?: string | null;
  submit_end_time?: string | null;
  problem_ids: string[];
  description: string;
  announcements?: Announcement[];
  registration_config?: RegistrationConfig;
}

export interface WorkflowStep {
  name: string;
  image?: string;
  root?: boolean;
  timeout?: number;
  show: boolean;
  steps: string[][];
  mounts?: any[];
  network?: boolean;
  resources?: StepResources;
  scheduling?: StepScheduling;
}

export interface StepResources {
  cpu_request?: string;
  cpu_limit?: string;
  memory_request?: string;
  memory_limit?: string;
  gpu_count?: number;
  gpu_resource?: string;
}

export interface StepScheduling {
  node_selector?: Record<string, string>;
  node_affinity?: NodeAffinityTerm[];
  tolerations?: Toleration[];
  priority_class_name?: string;
  runtime_class_name?: string;
}

export interface NodeAffinityTerm {
  key: string;
  operator: string;
  values?: string[];
}

export interface Toleration {
  key: string;
  operator: string;
  value?: string;
  effect?: string;
}

export interface ScoreConfig {
  max_performance_score: number;
  mode: string;
}

export interface DeadlineOverride {
  tags: string[];
  end_time: string;
}

export interface Problem {
    id: string;
    name: string;
    starttime: string;
    endtime: string;
    submit_start_time?: string | null;
    submit_end_time?: string | null;
    level: Level;
    cluster: string;
    max_submissions?: number;
    upload: {
        max_num: number;
        max_size: number;
        upload_form?: boolean;
        upload_files?: string[];
        editor?: boolean;
        editor_files?: string[];
    };
    score: {
      max_performance_score: number;
      mode: string;
    }
    workflow: WorkflowStep[];
    deadline_overrides?: DeadlineOverride[];
    effective_end_time?: string | null;
    description: string;
}

export interface Container {
  id: string;
  submission_id: string;
  image: string;
  status: Status;
  exit_code: number;
  started_at: string;
  finished_at: string;
  log_file_path: string;
  user_id: string;
  user?: User;
  CreatedAt: string;
  UpdatedAt: string;
}

export interface ProblemForSubmission {
  id: string;
  name: string;
  workflow: WorkflowStep[];
  score: ScoreConfig;
}

export interface Submission {
  id: string;
  CreatedAt: string;
  UpdatedAt: string;
  problem_id: string;
  user_id: string;
  user: User;
  status: Status;
  current_step: number;
  cluster: string;
  node: string;
  score: number;
  performance: number;
  info: { [key: string]: any };
  is_valid: boolean;
  problem?: ProblemForSubmission;
  containers: Container[];
}

export interface LeaderboardEntry {
  user_id: string;
  username: string;
  nickname: string;
  avatar_url: string;
  tags: string;
  disable_rank: boolean;
  total_score: number;
  problem_scores: Record<string, number>;
}

export interface ScoreHistoryPoint {
  time: string;
  score: number;
  problem_id: string;
}

export interface TrendEntry {
  user_id: string;
  username: string;
  nickname: string;
  history: ScoreHistoryPoint[];
}

export interface Attempts {
    limit: number | null;
    used: number;
    remaining: number | null;
}

export interface AuthStatus {
  local_auth_enabled: boolean;
}

export interface LinkItem {
    name: string;
    url: string;
}

export interface PaginatedResponse<T> {
  items: T[];
  total_items: number;
  total_pages: number;
  current_page: number;
  per_page: number;
}

export interface UserBestScore {
  ID: number;
  UserID: string;
  ContestID: string;
  ProblemID: string;
  Score: number;
  Performance: number;
  SubmissionID: string;
  SubmissionCount: number;
  LastScoreTime: string;
}

export interface AssetFile {
    name: string;
    path: string;
    is_dir: boolean;
    size: number;
    mod_time: string;
}

export interface ClusterRow {
  name: string;
  kubeconfig: string;
  context: string;
  namespace: string;
  concurrency: number;
  heartbeat_ttl: number;
  queue_mode: string;
}

export interface ClusterNodePool {
  cluster_name: string;
  pool_name: string;
  node_selector: Record<string, string>;
  cpu: number;
  memory: number;
  is_paused: boolean;
}

export interface PoolState {
  Name: string;
  NodeSelector: Record<string, string>;
  CPU: number;
  Memory: number;
  IsPaused: boolean;
}

export interface ClusterStateSnapshot {
  Name: string;
  Namespace: string;
  Pools: Record<string, PoolState>;
  MPIEnabled: boolean;
  QueueLength: number;
  Concurrency: number;
  QueueMode: string;
}

export interface ClusterStatusResponse {
  resource_status: Record<string, ClusterStateSnapshot>;
  queue_lengths: Record<string, number>;
}

export interface DevPodTemplate {
  id: string;
  name: string;
  cluster_name: string;
  image: string;
  shell: string;
  cores: number;
  memory: number;
  gpu_count: number;
  gpu_resource: string;
  node_selector: Record<string, string>;
  tolerations: any[];
  allowed_tags: string[];
  default_per_user: number;
  default_global: number;
}

export interface DevPodInstance {
  name: string;
  template: string;
  phase: string;
  running: boolean;
  endpoint: string;
  ssh_command: string;
  created_at: string;
}

export interface DevPodGateway { host: string; port: number; }

export interface AdminDevPodInstance {
  name: string;
  owner: string;
  template: string;
  cluster_name: string;
  namespace: string;
  phase: string;
  running: boolean;
  endpoint: string;
  message: string;
  created_at: string;
}

export interface AdminDevPodListResponse {
  items: AdminDevPodInstance[];
  warnings: string[];
}

export interface DevPodListResponse {
  items: DevPodInstance[];
  gateway: DevPodGateway;
  max_per_user: number;
}
