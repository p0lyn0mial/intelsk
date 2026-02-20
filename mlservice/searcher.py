"""Search CLIP embeddings in SQLite by text query."""

import sqlite3

import numpy as np

from clip_encoder import CLIPEncoder


class Searcher:
    """Ranks stored CLIP embeddings against a text query using cosine similarity."""

    def __init__(self, encoder: CLIPEncoder):
        self.encoder = encoder

    def search_by_text(
        self,
        db_path: str,
        text: str,
        camera_ids: list[str] | None = None,
        start_time: str | None = None,
        end_time: str | None = None,
        limit: int = 20,
        min_score: float = 0.18,
    ) -> list[dict]:
        """Encode text and rank stored CLIP embeddings by cosine similarity."""
        text_emb = self.encoder.encode_text(text)

        conn = sqlite3.connect(db_path)
        try:
            rows = self._load_clip_embeddings(conn, camera_ids, start_time, end_time)
        finally:
            conn.close()

        if not rows:
            return []

        # Build matrix of embeddings and compute similarities
        ids, embeddings, metadata = [], [], []
        for row in rows:
            ids.append(row[0])
            embeddings.append(np.frombuffer(row[1], dtype=np.float32))
            metadata.append({
                "camera_id": row[2],
                "timestamp": row[3],
                "frame_path": row[4],
                "source_video": row[5],
            })

        emb_matrix = np.stack(embeddings)
        # Cosine similarity = dot product (embeddings are L2-normalized)
        scores = emb_matrix @ text_emb

        # Rank by descending score, drop anything below the relevance threshold
        ranked_indices = np.argsort(scores)[::-1][:limit]

        results = []
        for idx in ranked_indices:
            if scores[idx] < min_score:
                break
            results.append({
                "id": ids[idx],
                "frame_path": metadata[idx]["frame_path"],
                "camera_id": metadata[idx]["camera_id"],
                "timestamp": metadata[idx]["timestamp"],
                "source_video": metadata[idx]["source_video"],
                "score": float(scores[idx]),
            })
        return results

    @staticmethod
    def _load_clip_embeddings(
        conn: sqlite3.Connection,
        camera_ids: list[str] | None = None,
        start_time: str | None = None,
        end_time: str | None = None,
    ) -> list[tuple]:
        """Load CLIP embeddings from SQLite with optional camera/time filtering."""
        query = "SELECT id, embedding, camera_id, timestamp, frame_path, source_video FROM clip_embeddings"
        conditions = []
        params = []

        if camera_ids:
            placeholders = ",".join("?" for _ in camera_ids)
            conditions.append(f"camera_id IN ({placeholders})")
            params.extend(camera_ids)
        if start_time:
            conditions.append("timestamp >= ?")
            params.append(start_time)
        if end_time:
            # If end_time is a date-only string (no "T"), append end-of-day
            # so that timestamps like "2026-02-20T14:00:00" are included.
            if "T" not in end_time:
                end_time = end_time + "T23:59:59"
            conditions.append("timestamp <= ?")
            params.append(end_time)

        if conditions:
            query += " WHERE " + " AND ".join(conditions)

        cursor = conn.execute(query, params)
        return cursor.fetchall()
