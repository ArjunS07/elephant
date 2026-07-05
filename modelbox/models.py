"""The two models and the inference calls over them. Loaded on first use and
kept resident. Nothing here touches the database."""

from sentence_transformers import SentenceTransformer, CrossEncoder

from config import DEVICE, EMBED_MODEL, RERANK_MODEL

# bge asks for this prefix on the query side only; passages are embedded as-is.
QUERY_PREFIX = "Represent this sentence for searching relevant passages: "

_embedder = None
_reranker = None


def _get_embedder():
    global _embedder
    if _embedder is None:
        _embedder = SentenceTransformer(EMBED_MODEL, device=DEVICE)
    return _embedder


def _get_reranker():
    global _reranker
    if _reranker is None:
        _reranker = CrossEncoder(RERANK_MODEL, device=DEVICE)
    return _reranker


def embed(texts, is_query):
    """Return one normalized 768-dim vector per text. Normalized so a dot product
    is cosine similarity, matching the halfvec_cosine_ops index."""
    if is_query:
        texts = [QUERY_PREFIX + t for t in texts]
    vectors = _get_embedder().encode(texts, normalize_embeddings=True)
    return vectors.tolist()


def rerank(query, passages):
    """Score each passage against the query with the cross-encoder. Higher is more
    relevant; scores are not normalized, only meaningful as a ranking."""
    scores = _get_reranker().predict([(query, p) for p in passages])
    return scores.tolist()


def warm():
    _get_embedder()
    _get_reranker()
