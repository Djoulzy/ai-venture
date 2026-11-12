# Exemple de catalogue

`catalogue.json` contient dix fiches bilingues et neuf images. Les sprites ont un fond transparent. Les images sont de simples formes géométriques de démonstration produites par `generate.go` ; elles peuvent être remplacées par vos propres illustrations.

```sh
go run data/catalogue/demo/generate.go
make build
./bin/catalogue import -source data/catalogue/demo -db catalogue.sqlite
```

Le générateur est exécuté explicitement depuis la racine du projet ; il ne fait pas partie de l'import ou des tests et ne télécharge rien. Les anciens scénarios illustrés du phare ont été retirés. Pour lancer une scène illustrée complète, utiliser [Providence](../providence/README.md). Voir `docs/catalogue.md` pour le format complet.
