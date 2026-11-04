# Conventions de développement

- Le projet est un jeu de terminal Go, en anglais ou français (`-lang en|fr`, anglais par défaut), sans dépendance Go externe pour le prototype.
- Garder les prompts, descriptions et commandes cohérents avec la langue choisie. Centraliser les messages traduits dans `internal/locale` ; utiliser `locale.Errorf` pour les erreurs applicatives et conserver leurs causes. Ne jamais traduire arbitrairement les textes du joueur ou du modèle. La documentation de développement peut rester en français.
- Tester les parcours anglais et français ; les variantes du scénario de démonstration possèdent leurs propres identifiants et sauvegardes. Un scénario externe conserve les textes fournis dans son fichier.
- `internal/world` définit et valide les scénarios ; `internal/engine` est seul responsable des règles et de l'état courant.
- Le LLM propose une seule action typée. Ne jamais exécuter du code, un chemin de fichier ou un changement d'état provenant du modèle.
- Toute nouvelle action doit être validée côté moteur, y compris lorsqu'elle provient d'un autre interpréteur que le LLM.
- Appliquer les effets sur une copie, vérifier les invariants, puis remplacer l'état. Un échec ne modifie rien.
- Ne transmettre à l'inférence que la vue visible et des résultats validés, jamais les règles secrètes ni une référence mutable au moteur.
- Isoler les fournisseurs d'inférence derrière `inference.Client`. Propager `context.Context`, limiter les réponses et les délais HTTP.
- Garder la gestion des processus dans `internal/modelruntime`. Réutiliser les services existants sans jamais les arrêter. Seule une connexion refusée autorise le démarrage d'un service local.
- Démarrer les binaires avec `os/exec` et des arguments séparés, sans shell. Borner le démarrage, les journaux et l'arrêt ; attendre les processus lancés et nettoyer leurs descendants sous macOS/Linux.
- Tester les adaptateurs et le cycle de vie avec des serveurs et sous-processus de test ; ne pas télécharger de modèles ni installer de binaires durant les tests.
- Décoder strictement les entrées JSON. Maintenir le schéma des actions et sa validation Go ensemble.
- Les scénarios générés devront passer par la même validation que les fichiers fixes. Ne pas contourner les préconditions pour faciliter une génération.
- Les sauvegardes sont versionnées, associées à l'empreinte du scénario et écrites atomiquement. Valider avant de remplacer l'état courant.
- Ajouter des tests de comportement pour les règles, les refus sans mutation, les entrées LLM invalides et la persistance. Les tests ordinaires ne doivent pas nécessiter Ollama ou Internet.
- Exécuter `gofmt`, `go test ./...`, `go test -race ./...` et `go vet ./...` avant livraison d'une modification fonctionnelle.
- Documenter les changements de commandes, de format JSON et d'architecture. Ne pas committer de sauvegardes, binaires ou modèles.
