# Conventions de développement

- Le projet est un jeu de terminal Go, en anglais ou français (`-lang en|fr`, anglais par défaut), avec Go 1.26 minimum et le pilote SQLite sans CGO `modernc.org/sqlite`.
- Garder les prompts, descriptions et commandes cohérents avec la langue choisie. Centraliser les messages traduits dans `internal/locale` ; utiliser `locale.Errorf` pour les erreurs applicatives et conserver leurs causes. Ne jamais traduire arbitrairement les textes du joueur ou du modèle. La documentation de développement peut rester en français.
- Tester les parcours anglais et français ; les variantes du scénario de démonstration possèdent leurs propres identifiants et sauvegardes. Un scénario externe conserve les textes fournis dans son fichier.
- `internal/world` définit et valide les scénarios ; `internal/engine` est seul responsable des règles et de l'état courant.
- Le LLM propose une seule action typée. Ne jamais exécuter du code, un chemin de fichier ou un changement d'état provenant du modèle.
- Toute nouvelle action doit être validée côté moteur, y compris lorsqu'elle provient d'un autre interpréteur que le LLM.
- Appliquer les effets sur une copie, vérifier les invariants, puis remplacer l'état. Un échec ne modifie rien.
- Ne transmettre à l'inférence que la vue visible et des résultats validés, jamais les règles secrètes ni une référence mutable au moteur.
- Isoler les fournisseurs d'inférence derrière `inference.Client`. Propager `context.Context`, limiter les réponses et les délais HTTP.
- Garder la gestion des processus dans `internal/modelruntime`. Réutiliser les services existants sans jamais les arrêter. Seule une connexion refusée autorise le démarrage d'un service local.
- Avec llama.cpp, dériver l'alias par défaut du GGUF et vérifier aussi le chemin annoncé par `/props` si `-gguf` est fourni. Un alias identique ne suffit pas à valider le fichier chargé. Réserver `OLLAMA_MODEL` au fournisseur Ollama.
- Démarrer les binaires avec `os/exec` et des arguments séparés, sans shell. Borner le démarrage, les journaux et l'arrêt ; attendre les processus lancés et nettoyer leurs descendants sous macOS/Linux.
- Tester les adaptateurs et le cycle de vie avec des serveurs et sous-processus de test ; ne pas télécharger de modèles ni installer de binaires durant les tests.
- Décoder strictement les entrées JSON. Maintenir le schéma des actions et sa validation Go ensemble.
- Les scénarios générés devront passer par la même validation que les fichiers fixes. Ne pas contourner les préconditions pour faciliter une génération.
- Les sauvegardes sont versionnées, associées à l'empreinte du scénario et écrites atomiquement. Valider avant de remplacer l'état courant.
- Ajouter des tests de comportement pour les règles, les refus sans mutation, les entrées LLM invalides et la persistance. Les tests ordinaires ne doivent pas nécessiter Ollama ou Internet.
- Exécuter `gofmt`, `go test ./...`, `go test -race ./...` et `go vet ./...` avant livraison d'une modification fonctionnelle.
- Documenter les changements de commandes, de format JSON et d'architecture. Ne pas committer de sauvegardes, binaires ou modèles.

- Le RAG est réservé à la narration. Le moteur possède les découvertes et le journal ; SQLite est un cache reconstructible, jamais une source d’état.
- N’indexer que `Engine.Memory()`, filtrée par découvertes et langue. Ne jamais indexer les règles, les conditions secrètes, le texte du joueur ou la narration générée.
- Remplacer le corpus du scénario et de la langue dans la même transaction que la recherche ; un rechargement ne doit jamais réintroduire une branche abandonnée. Borner le nombre et la taille des extraits.
- Les extraits retenus sont reconstruits depuis la copie approuvée par Go, par identifiant. Les tests RAG utilisent SQLite réelle et un LLM simulé ; couvrir les secrets, les langues, le rechargement et le repli sans RAG.
